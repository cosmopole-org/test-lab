package net_federation

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"kasper/src/abstract/adapters/file"
	"kasper/src/abstract/adapters/network"
	"kasper/src/abstract/adapters/signaler"
	"kasper/src/abstract/adapters/storage"
	iaction "kasper/src/abstract/models/action"
	"kasper/src/abstract/models/core"
	"kasper/src/abstract/models/input"
	"kasper/src/abstract/models/packet"
	"kasper/src/abstract/models/trx"
	inputs_invites "kasper/src/shell/api/inputs/invites"
	inputs_stores "kasper/src/shell/api/inputs/stores"
	outputs_stores "kasper/src/shell/api/outputs/stores"
	updates_stores "kasper/src/shell/api/updates/stores"
	"kasper/src/shell/utils/crypto"
	"kasper/src/shell/utils/future"
	"net"
	"strconv"
	"time"

	cmap "github.com/orcaman/concurrent-map/v2"
)

type FedPacketCallback struct {
	UserId        string
	Key           string
	Request       []byte
	Callback      func([]byte, int, error)
	UserRequestId string
}

type FedFileCallback struct {
	Callback      *cmap.ConcurrentMap[string, func(string, error)]
	UserRequestId string
}

type FedNet struct {
	app             core.ICore
	storage         storage.IStorage
	file            file.IFile
	signaler        signaler.ISignaler
	Gateway         *Tcp
	packetCallbacks *cmap.ConcurrentMap[string, *FedPacketCallback]
	Port            int
}

func FirstStageBackFill(core core.ICore) *FedNet {
	m := cmap.New[*FedPacketCallback]()
	return &FedNet{app: core, packetCallbacks: &m}
}

func (fed *FedNet) Listen(port int, tlsConfig *tls.Config) {
	fed.Port = port
	future.Async(func() {
		fed.Gateway.Listen(port, tlsConfig)
	}, false)
}

func (fed *FedNet) SecondStageForFill(storage storage.IStorage, file file.IFile, signaler signaler.ISignaler) network.IFederation {
	fed.Gateway = NewTcp(fed.app)
	fed.storage = storage
	fed.file = file
	fed.signaler = signaler
	fed.Gateway.InjectBridge(func(socket *Socket, ip string, pack packet.OriginPacket) {
		hostName := ""
		for _, peer := range fed.app.Tools().Network().Chain().Peers() {
			if peer == ip {
				fed.app.ModifyState(true, func(trx trx.ITrx) error {
					hostName = trx.GetLink("NodeIpToHost::" + ip)
					return nil
				})
				break
			}
		}
		if hostName != "" {
			fed.HandlePacket(socket, hostName, pack)
		} else {
		}
	})
	return fed
}

func ParseInput[T input.IInput](i string) (input.IInput, error) {
	body := new(T)
	err := json.Unmarshal([]byte(i), body)
	if err != nil {
		return nil, errors.New("invalid input format")
	}
	return *body, nil
}

func (fed *FedNet) HandlePacket(socket *Socket, channelId string, payload packet.OriginPacket) {
	if payload.Type == "response" {
		cb, ok := fed.packetCallbacks.Get(payload.RequestId)
		if ok {
			if payload.ResCode == 0 {
				if cb.Key == "/invites/accept" || cb.Key == "/stores/join" {
					userId := ""
					storeId := ""
					if cb.Key == "/invites/accept" {
						var memberRes inputs_invites.AcceptInput
						err2 := json.Unmarshal(cb.Request, &memberRes)
						if err2 != nil {
							return
						}
						userId = cb.UserId
						storeId = memberRes.StoreId
					} else if cb.Key == "/stores/join" {
						var memberRes inputs_stores.JoinInput
						err2 := json.Unmarshal(cb.Request, &memberRes)
						if err2 != nil {
							return
						}
						userId = cb.UserId
						storeId = memberRes.StoreId
					}
					if storeId != "" {
						fed.app.ModifyState(false, func(trx trx.ITrx) error {
							trx.PutLink("onaccess::"+storeId+"::"+userId, "true")
							trx.PutLink("hasaccess::"+userId+"::"+storeId, "true")
							return nil
						})
						fed.signaler.JoinGroup(storeId, userId)
					}
				} else if cb.Key == "/stores/create" {
					var spaceOut outputs_stores.CreateOutput
					err3 := json.Unmarshal(payload.Binary, &spaceOut)
					if err3 != nil {
						return
					}
					fed.app.ModifyState(false, func(trx trx.ITrx) error {
						spaceOut.Store.Store.Pull(trx)
						trx.PutLink("onaccess::"+spaceOut.Store.Store.Id+"::"+cb.UserId, "true")
						trx.PutLink("hasaccess::"+cb.UserId+"::"+spaceOut.Store.Store.Id, "true")
						return nil
					})
					fed.signaler.JoinGroup(spaceOut.Store.Store.Id, cb.UserId)
				}
			}
			fed.packetCallbacks.Remove(payload.RequestId)
			if payload.ResCode != 0 {
				errPack := payload.Binary
				errObj := packet.Error{}
				json.Unmarshal([]byte(errPack), &errObj)
				err := errors.New(errObj.Message)
				cb.Callback([]byte(""), 1, err)
			} else {
				cb.Callback(payload.Binary, 0, nil)
			}
		}
	} else if payload.Type == "update" {
		reactToUpdate := func(key string, data string) {
			if key == "stores/update" {
				tc := updates_stores.Update{}
				err := json.Unmarshal([]byte(data), &tc)
				if err != nil {
					return
				}
				fed.app.ModifyState(false, func(trx trx.ITrx) error {
					tc.Store.Push(trx)
					return nil
				})
			} else if key == "stores/delete" {
				tc := updates_stores.Delete{}
				err := json.Unmarshal([]byte(data), &tc)
				if err != nil {
					return
				}
				fed.app.ModifyState(false, func(trx trx.ITrx) error {
					trx.DelKey("obj::Store::" + tc.Store.Id)
					return nil
				})
			} else if key == "stores/addMember" {
				tc := updates_stores.AddMember{}
				err := json.Unmarshal([]byte(data), &tc)
				if err != nil {
					return
				}
				fed.app.ModifyState(false, func(trx trx.ITrx) error {
					trx.PutLink("onaccess::"+tc.StoreId+"::"+tc.User.Id, "true")
					trx.PutLink("hasaccess::"+tc.User.Id+"::"+tc.StoreId, "true")
					return nil
				})
			} else if key == "stores/removeMember" {
				tc := updates_stores.AddMember{}
				err := json.Unmarshal([]byte(data), &tc)
				if err != nil {
					return
				}
				fed.app.ModifyState(false, func(trx trx.ITrx) error {
					trx.DelKey("link::onaccess::" + tc.StoreId + "::" + tc.User.Id)
					trx.DelKey("link::hasaccess::" + tc.User.Id + "::" + tc.StoreId)
					return nil
				})
			} else if key == "stores/updateMember" {
				tc := updates_stores.UpdateMember{}
				err := json.Unmarshal([]byte(data), &tc)
				if err != nil {
					return
				}
				fed.app.ModifyState(false, func(trx trx.ITrx) error {
					trx.PutJson("member_"+tc.StoreId+"_"+tc.User.Id, "meta", tc.Metadata, false)
					return nil
				})
			} else if key == "stores/join" {
				tc := updates_stores.Join{}
				err := json.Unmarshal([]byte(data), &tc)
				if err != nil {
					return
				}
				fed.app.ModifyState(false, func(trx trx.ITrx) error {
					trx.PutLink("onaccess::"+tc.StoreId+"::"+tc.User.Id, "true")
					trx.PutLink("hasaccess::"+tc.User.Id+"::"+tc.StoreId, "true")
					return nil
				})
			}
		}
		if payload.StoreId == "" {
			reactToUpdate(payload.Key, string(payload.Binary))
			fed.signaler.SignalUser(payload.Key, payload.UserId, payload.Binary, false)
		} else {
			reactToUpdate(payload.Key, string(payload.Binary))
			fed.signaler.SignalGroup(payload.Key, payload.StoreId, payload.Binary, false, payload.Exceptions)
		}
	} else if payload.Type == "request" {
		action := fed.app.Actor().FetchAction(payload.Key)
		if action == nil {
			fed.SendFedResponse(channelId, payload.RequestId, 1, packet.BuildErrorJson("action not found"))
		}
		input, err := action.(iaction.ISecureAction).ParseInput("fed", payload.Binary)
		if err != nil {
			fed.SendFedResponse(channelId, payload.RequestId, 1, packet.BuildErrorJson("input could not be parsed"))
		}
		_, res, err := action.(iaction.ISecureAction).SecurelyActFed(payload.UserId, payload.Binary, payload.Signature, input)
		if err != nil {
			fed.SendFedResponse(channelId, payload.RequestId, 1, packet.BuildErrorJson(err.Error()))
			return
		}
		fed.SendFedResponse(channelId, payload.RequestId, 0, res)
	}
}

func (fed *FedNet) SendFedRequest(destOrg string, requestId string, userId string, path string, payload []byte, signature string) {
	ipAddr := ""
	ips, _ := net.LookupIP(destOrg)
	for _, ip := range ips {
		if ipv4 := ip.To4(); ipv4 != nil {
			ipAddr = ipv4.String()
			break
		}
	}
	ok := false
	for _, peer := range fed.app.Tools().Network().Chain().Peers() {
		if peer == ipAddr {
			ok = true
			break
		}
	}
	if ok {
		address := destOrg + ":" + strconv.Itoa(fed.Port)
		s := fed.Gateway.NewSocket(address)
		if s == nil {
			return
		}
		defer s.Conn.Close()
		s.writeRequest(requestId, userId, path, payload, signature)
	}
}

func (fed *FedNet) SendFedResponse(destOrg string, requestId string, resCode int, res any) {
	ipAddr := ""
	ips, _ := net.LookupIP(destOrg)
	for _, ip := range ips {
		if ipv4 := ip.To4(); ipv4 != nil {
			ipAddr = ipv4.String()
			break
		}
	}
	ok := false
	for _, peer := range fed.app.Tools().Network().Chain().Peers() {
		if peer == ipAddr {
			ok = true
			break
		}
	}
	if ok {
		address := destOrg + ":" + strconv.Itoa(fed.Port)
		s := fed.Gateway.NewSocket(address)
		if s == nil {
			return
		}
		defer s.Conn.Close()
		s.writeResponse(requestId, resCode, res, false)
	}
}

func (fed *FedNet) SendFedUpdate(destOrg string, key string, updatePack any, targetType string, targetIdVal string, exceptions []string) {
	ipAddr := ""
	ips, _ := net.LookupIP(destOrg)
	for _, ip := range ips {
		if ipv4 := ip.To4(); ipv4 != nil {
			ipAddr = ipv4.String()
			break
		}
	}
	ok := false
	for _, peer := range fed.app.Tools().Network().Chain().Peers() {
		if peer == ipAddr {
			ok = true
			break
		}
	}
	if ok {
		address := destOrg + ":" + strconv.Itoa(fed.Port)
		s := fed.Gateway.NewSocket(address)
		if s == nil {
			return
		}
		defer s.Conn.Close()
		s.writeUpdate(key, updatePack, targetType, targetIdVal, exceptions, false)
	}
}

func (fed *FedNet) SendFedRequestByCallback(destOrg string, requestId string, userId string, path string, payload []byte, signature string, callback func([]byte, int, error)) {
	ipAddr := ""
	ips, _ := net.LookupIP(destOrg)
	for _, ip := range ips {
		if ipv4 := ip.To4(); ipv4 != nil {
			ipAddr = ipv4.String()
			break
		}
	}
	ok := false
	for _, peer := range fed.app.Tools().Network().Chain().Peers() {
		if peer == ipAddr {
			ok = true
			break
		}
	}
	if ok {
		callbackId := crypto.SecureUniqueString()
		cb := &FedPacketCallback{Callback: callback, Key: path, UserRequestId: requestId, Request: payload, UserId: userId}
		fed.packetCallbacks.Set(callbackId, cb)
		future.Async(func() {
			time.Sleep(time.Duration(120) * time.Second)
			cb, ok := fed.packetCallbacks.Get(callbackId)
			if ok {
				fed.packetCallbacks.Remove(callbackId)
				cb.Callback([]byte(""), 0, errors.New("federation callback timeout"))
			}
		}, false)
		address := destOrg + ":" + strconv.Itoa(fed.Port)
		s := fed.Gateway.NewSocket(address)
		if s == nil {
			return
		}
		defer s.Conn.Close()
		s.writeRequest(callbackId, userId, path, payload, signature)
	}
}
