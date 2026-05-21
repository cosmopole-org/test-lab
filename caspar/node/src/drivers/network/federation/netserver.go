package net_federation

import (
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"kasper/src/abstract/models/core"
	"kasper/src/shell/utils/crypto"
	"kasper/src/shell/utils/future"
	"net"
	"strings"
	"sync"

	packetmodel "kasper/src/abstract/models/packet"

	cmap "github.com/orcaman/concurrent-map/v2"
)

type Socket struct {
	Id     string
	Lock   sync.Mutex
	Conn   net.Conn
	Buffer [][]byte
	Ack    bool
	app    core.ICore
	server *Tcp
}

type FedApi func(socket *Socket, srcIp string, packet packetmodel.OriginPacket)

type Tcp struct {
	app     core.ICore
	bridge  FedApi
	sockets *cmap.ConcurrentMap[string, *Socket]
}

func (t *Tcp) InjectBridge(bridge FedApi) {
	t.bridge = bridge
}

func (t *Tcp) Listen(port int, tlsConfig *tls.Config) {
	future.Async(func() {
		ln, err := tls.Listen("tcp", fmt.Sprintf(":%d", port), tlsConfig)
		if err != nil {
			panic(fmt.Sprintf("failed to listen: %v", err))
		}
		defer ln.Close()

		for {
			conn, err := ln.Accept()
			if err != nil {
				fmt.Println(err)
				continue
			}
			t.handleConnection(conn)
		}
	}, true)
}

func (t *Tcp) listenForPackets(socket *Socket) {
	defer func() {
		t.sockets.Remove(socket.Id)
		socket.Conn.Close()
	}()
	lenBuf := make([]byte, 4)
	buf := make([]byte, 1024)
	nextBuf := make([]byte, 2048)
	readCount := 0
	oldReadCount := 0
	enough := false
	beginning := true
	length := 0
	readLength := 0
	remainedReadLength := 0
	var readData []byte
	for {
		if !enough {
			var err error
			readLength, err = socket.Conn.Read(buf)
			if err != nil {
				return
			}
			func() {
				socket.Lock.Lock()
				defer socket.Lock.Unlock()

				readCount += readLength
				copy(nextBuf[remainedReadLength:remainedReadLength+readLength], buf[0:readLength])
				remainedReadLength += readLength

			}()
		}

		if beginning {
			if readCount >= 4 {
				func() {
					socket.Lock.Lock()
					defer socket.Lock.Unlock()
					copy(lenBuf, nextBuf[0:4])
					remainedReadLength -= 4
					copy(nextBuf[0:remainedReadLength], nextBuf[4:remainedReadLength+4])
					length = int(binary.LittleEndian.Uint32(lenBuf))
					readData = make([]byte, length)
					readCount -= 4
					beginning = false
					enough = true

				}()

			} else {
				enough = false
			}
		} else {
			if remainedReadLength == 0 {
				enough = false
			} else if readCount >= length {
				func() {
					socket.Lock.Lock()
					defer socket.Lock.Unlock()
					copy(readData[oldReadCount:length], nextBuf[0:length-oldReadCount])
					readCount -= length
					copy(nextBuf[0:readCount], nextBuf[length-oldReadCount:(length-oldReadCount)+readCount])
					remainedReadLength = readCount
					packet := make([]byte, length)
					copy(packet, readData)
					oldReadCount = 0
					enough = true
					beginning = true

					future.Async(func() {
						defer func() {
							if err := recover(); err != nil {
							}
						}()
						socket.processPacket(packet)
					}, false)
				}()
			} else {
				func() {
					socket.Lock.Lock()
					defer socket.Lock.Unlock()

					copy(readData[oldReadCount:oldReadCount+(readCount-oldReadCount)], nextBuf[0:readCount-oldReadCount])
					remainedReadLength = 0
					oldReadCount = readCount
					enough = true

				}()
			}
		}
	}
}

func (t *Tcp) handleConnection(conn net.Conn) *Socket {
	socket := &Socket{Id: crypto.SecureUniqueString(), server: t, Buffer: [][]byte{}, Conn: conn, app: t.app, Ack: true}
	t.sockets.Set(strings.Split(conn.RemoteAddr().String(), ":")[0], socket)
	future.Async(func() {
		t.listenForPackets(socket)
	}, false)
	return socket
}

func (t *Tcp) NewSocket(destAddress string) *Socket {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: false,
		ServerName:         strings.Split(destAddress, ":")[0],
	}
	conn, err := tls.Dial("tcp", destAddress, tlsConfig)
	if err != nil {
	}
	return t.handleConnection(conn)
}

func (t *Socket) writeRequest(requestId string, userId string, path string, payload []byte, signature string) {

	signBytes := []byte(signature)
	signLenBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(signLenBytes[:], uint32(len(signBytes)))

	userIdBytes := []byte(userId)
	userIdLenBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(userIdLenBytes[:], uint32(len(userIdBytes)))

	pathBytes := []byte(path)
	pathLenBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(pathLenBytes[:], uint32(len(pathBytes)))

	pidBytes := []byte(requestId)
	pidLenBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(pidLenBytes[:], uint32(len(pidBytes)))

	packet := make([]byte, 1+len(signLenBytes)+len(signBytes)+len(userIdLenBytes)+len(userIdBytes)+len(pathLenBytes)+len(pathBytes)+len(pidLenBytes)+len(pidBytes)+len(payload))

	pointer := 1

	packet[0] = 0x03
	copy(packet[pointer:pointer+len(signLenBytes)], signLenBytes[:])
	pointer += len(signLenBytes)
	copy(packet[pointer:pointer+len(signBytes)], signBytes[:])
	pointer += len(signBytes)

	copy(packet[pointer:pointer+len(userIdLenBytes)], userIdLenBytes[:])
	pointer += len(userIdLenBytes)
	copy(packet[pointer:pointer+len(userIdBytes)], userIdBytes[:])
	pointer += len(userIdBytes)

	copy(packet[pointer:pointer+len(pathLenBytes)], pathLenBytes[:])
	pointer += len(pathLenBytes)
	copy(packet[pointer:pointer+len(pathBytes)], pathBytes[:])
	pointer += len(pathBytes)

	copy(packet[pointer:pointer+len(pidLenBytes)], pidLenBytes[:])
	pointer += len(pidLenBytes)
	copy(packet[pointer:pointer+len(pidBytes)], pidBytes[:])
	pointer += len(pidBytes)

	copy(packet[pointer:pointer+len(payload)], payload[:])
	pointer += len(payload)

	t.Lock.Lock()
	defer t.Lock.Unlock()

	t.Buffer = append(t.Buffer, packet)
	t.pushBuffer()
}

func (t *Socket) writeUpdate(key string, updatePack any, targetType string, targetIdVal string, exceptions []string, writeRaw bool) {

	targetId := targetType + "::" + targetIdVal

	keyBytes := []byte(key)
	keyLenBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(keyLenBytes[:], uint32(len(keyBytes)))

	var b3 []byte
	var signBytes []byte
	if writeRaw {
		b3 = updatePack.([]byte)
	} else {
		var err error
		b3, err = json.Marshal(updatePack)
		if err != nil {
			return
		}
		signBytes = []byte(t.app.SignPacket(b3))
	}

	signLenBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(signLenBytes[:], uint32(len(signBytes)))

	targetIdBytes := []byte(targetId)
	targetIdLenBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(targetIdLenBytes[:], uint32(len(targetIdBytes)))

	excepBytes, err := json.Marshal(exceptions)
	if err != nil {
		return
	}
	excepLenBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(excepLenBytes[:], uint32(len(excepBytes)))

	packet := make([]byte, 1+len(signLenBytes)+len(signBytes)+len(targetIdLenBytes)+len(targetIdBytes)+len(excepLenBytes)+len(excepBytes)+len(keyLenBytes)+len(keyBytes)+len(b3))

	pointer := 1

	packet[0] = 0x01
	copy(packet[pointer:pointer+len(signLenBytes)], signLenBytes[:])
	pointer += len(signLenBytes)
	copy(packet[pointer:pointer+len(signBytes)], signBytes[:])
	pointer += len(signBytes)

	copy(packet[pointer:pointer+len(targetIdLenBytes)], targetIdLenBytes[:])
	pointer += len(targetIdLenBytes)
	copy(packet[pointer:pointer+len(targetIdBytes)], targetIdBytes[:])
	pointer += len(targetIdBytes)

	copy(packet[pointer:pointer+len(excepLenBytes)], excepLenBytes[:])
	pointer += len(excepLenBytes)
	copy(packet[pointer:pointer+len(excepBytes)], excepBytes[:])
	pointer += len(excepBytes)

	copy(packet[pointer:pointer+len(keyLenBytes)], keyLenBytes[:])
	pointer += len(keyLenBytes)
	copy(packet[pointer:pointer+len(keyBytes)], keyBytes[:])
	pointer += len(keyBytes)

	copy(packet[pointer:pointer+len(b3)], b3[:])
	pointer += len(b3)

	t.Lock.Lock()
	defer t.Lock.Unlock()

	t.Buffer = append(t.Buffer, packet)
	t.pushBuffer()
}

func (t *Socket) writeResponse(requestId string, resCode int, response any, writeRaw bool) {

	b1 := []byte(requestId)
	b1Len := make([]byte, 4)
	binary.BigEndian.PutUint32(b1Len, uint32(len(b1)))

	b2 := make([]byte, 4)
	binary.BigEndian.PutUint32(b2, uint32(resCode))

	var b3 []byte
	var b4 []byte
	if writeRaw {
		b3 = response.([]byte)
	} else {
		var err error
		b3, err = json.Marshal(response)
		if err != nil {
			return
		}
		b4 = []byte(t.app.SignPacket(b3))
	}
	b4Len := make([]byte, 4)
	binary.BigEndian.PutUint32(b4Len, uint32(len(b4)))

	packet := make([]byte, 1+len(b1Len)+len(b1)+len(b2)+len(b4Len)+len(b4)+len(b3))
	pointer := 1

	packet[0] = 0x02

	copy(packet[pointer:pointer+len(b1Len)], b1Len[:])
	pointer += len(b1Len)
	copy(packet[pointer:pointer+len(b1)], b1[:])
	pointer += len(b1)

	copy(packet[pointer:pointer+len(b2)], b2[:])
	pointer += len(b2)

	copy(packet[pointer:pointer+len(b4Len)], b4Len[:])
	pointer += len(b4Len)
	copy(packet[pointer:pointer+len(b4)], b4[:])
	pointer += len(b4)

	copy(packet[pointer:pointer+len(b3)], b3[:])
	pointer += len(b3)

	t.Lock.Lock()
	defer t.Lock.Unlock()

	t.Buffer = append(t.Buffer, packet)
	t.pushBuffer()
}

func (t *Socket) pushBuffer() {
	if t.Ack {
		if len(t.Buffer) > 0 {
			t.Ack = false
			packetLen := make([]byte, 4)
			binary.BigEndian.PutUint32(packetLen, uint32(len(t.Buffer[0])))
			_, err := t.Conn.Write(packetLen)
			if err != nil {
				t.Ack = true
			}
			_, err = t.Conn.Write(t.Buffer[0])
			if err != nil {
				t.Ack = true
			}
		}
	}
}

func (t *Socket) processPacket(packet []byte) {
	if len(packet) == len([]byte("packet_received")) && string(packet) == "packet_received" {
		send := func() {
			t.Lock.Lock()
			defer t.Lock.Unlock()
			t.Ack = true
			if len(t.Buffer) > 0 {
				t.Buffer = t.Buffer[1:]
				if len(t.Buffer) > 0 {
					t.Ack = false
					_, err := t.Conn.Write(t.Buffer[0])
					if err != nil {
						t.Ack = true
					}
				}
			}
		}
		send()
		return
	}
	typ := ""
	switch packet[0] {
	case 0x01:
		{
			typ = "update"
			break
		}
	case 0x02:
		{
			typ = "response"
			break
		}
	case 0x03:
		{
			typ = "request"
			break
		}
	}
	var pack packetmodel.OriginPacket
	pointer := 1
	if typ == "request" {
		signatureLength := int(binary.BigEndian.Uint32(packet[pointer : pointer+4]))
		pointer += 4
		signature := string(packet[pointer : pointer+signatureLength])
		pointer += signatureLength
		userIdLength := int(binary.BigEndian.Uint32(packet[pointer : pointer+4]))
		pointer += 4
		userId := string(packet[pointer : pointer+userIdLength])
		pointer += userIdLength
		pathLength := int(binary.BigEndian.Uint32(packet[pointer : pointer+4]))
		pointer += 4
		path := string(packet[pointer : pointer+pathLength])
		pointer += pathLength
		packetIdLength := int(binary.BigEndian.Uint32(packet[pointer : pointer+4]))
		pointer += 4
		packetId := string(packet[pointer : pointer+packetIdLength])
		pointer += packetIdLength
		payload := packet[pointer:]
		pack = packetmodel.OriginPacket{Type: typ, Key: path, UserId: userId, StoreId: "", ResCode: 0, Binary: payload, Signature: signature, RequestId: packetId, Exceptions: []string{}}
	} else if typ == "response" {
		packetIdLength := int(binary.BigEndian.Uint32(packet[pointer : pointer+4]))
		pointer += 4
		packetId := string(packet[pointer : pointer+packetIdLength])
		pointer += packetIdLength
		resCode := int(binary.BigEndian.Uint32(packet[pointer : pointer+4]))
		pointer += 4
		signatureLength := int(binary.BigEndian.Uint32(packet[pointer : pointer+4]))
		pointer += 4
		signature := string(packet[pointer : pointer+signatureLength])
		pointer += signatureLength
		payload := packet[pointer:]
		pack = packetmodel.OriginPacket{Type: typ, Key: "", UserId: "", StoreId: "", ResCode: resCode, Binary: payload, Signature: signature, RequestId: packetId, Exceptions: []string{}}
	} else if typ == "update" {
		signatureLength := int(binary.BigEndian.Uint32(packet[pointer : pointer+4]))
		pointer += 4
		signature := string(packet[pointer : pointer+signatureLength])
		pointer += signatureLength
		targetIdLength := int(binary.BigEndian.Uint32(packet[pointer : pointer+4]))
		pointer += 4
		targetId := string(packet[pointer : pointer+targetIdLength])
		pointer += targetIdLength
		exceptionsLength := int(binary.BigEndian.Uint32(packet[pointer : pointer+4]))
		pointer += 4
		exceptions := []string{}
		err := json.Unmarshal(packet[pointer:pointer+exceptionsLength], &exceptions)
		pointer += exceptionsLength
		if err != nil {
			return
		}
		keyLength := int(binary.BigEndian.Uint32(packet[pointer : pointer+4]))
		pointer += 4
		key := string(packet[pointer : pointer+keyLength])
		pointer += keyLength
		payload := packet[pointer:]
		idParts := strings.Split(targetId, "::")
		userId := ""
		storeId := ""
		if idParts[0] == "user" {
			userId = idParts[1]
		} else if idParts[0] == "store" {
			storeId = idParts[1]
		}
		pack = packetmodel.OriginPacket{Type: typ, Key: key, UserId: userId, StoreId: storeId, ResCode: 0, Binary: payload, Signature: signature, RequestId: "", Exceptions: exceptions}
	}

	t.server.bridge(t, strings.Split(t.Conn.RemoteAddr().String(), ":")[0], pack)
}

func NewTcp(app core.ICore) *Tcp {
	m := cmap.New[*Socket]()
	return &Tcp{app: app, sockets: &m}
}
