package actions_creature

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"kasper/src/abstract/models/action"
	"kasper/src/abstract/models/core"
	"kasper/src/abstract/models/trx"
	"kasper/src/abstract/state"
	"kasper/src/core/module/actor/model/base"
	mainstate "kasper/src/core/module/actor/model/state"
	inputs_creatures "kasper/src/shell/api/inputs/creatures"
	inputsusers "kasper/src/shell/api/inputs/users"
	"kasper/src/shell/api/model"
	outputsusers "kasper/src/shell/api/outputs/users"
	updates_stores "kasper/src/shell/api/updates/stores"
	"kasper/src/shell/utils/crypto"
	"kasper/src/shell/utils/future"
	"log"
	"os"
	"strings"
	"time"

	firebase "firebase.google.com/go/v4"
	"google.golang.org/api/option"
)

type Actions struct {
	App           core.ICore
	OauthCtx      context.Context
	firebaseApp   *firebase.App
	modelExtender map[string]map[string]action.ExtendedField
}

type lockedTokenStep struct {
	Amount     int64 `json:"amount"`
	UnlockAt   int64 `json:"unlockAt"`
	Consumed   bool  `json:"consumed"`
	ConsumedAt int64 `json:"consumedAt,omitempty"`
}

func asInt64(raw any) (int64, bool) {
	switch v := raw.(type) {
	case int:
		return int64(v), true
	case int64:
		return v, true
	case float64:
		return int64(v), true
	default:
		return 0, false
	}
}

func (a *Actions) initFirebase() {
	credPath := os.Getenv("FIREBASE_SERVICE_ACCOUNT")
	if credPath == "" {
		credPath = "/app/serviceAccounts.json"
	}
	if _, err := os.Stat(credPath); err != nil {
		log.Printf("Firebase credentials not found at %s; running in DEV mode without Firebase auth (login will use emailToken as raw email).\n", credPath)
		a.firebaseApp = nil
		return
	}
	opt := option.WithCredentialsFile(credPath)
	app, err := firebase.NewApp(context.Background(), nil, opt)
	if err != nil {
		log.Printf("Error initializing Firebase app: %v -- continuing in DEV mode\n", err)
		a.firebaseApp = nil
		return
	}
	a.firebaseApp = app
}

func Install(a *Actions, params ...any) error {
	a.OauthCtx = context.Background()
	a.initFirebase()
	if len(params) >= 1 {
		a.modelExtender = params[0].(map[string]map[string]action.ExtendedField)
	} else {
		a.modelExtender = map[string]map[string]action.ExtendedField{}
	}
	if _, ok := a.modelExtender["user"]; !ok {
		a.modelExtender["user"] = map[string]action.ExtendedField{}
	}
	return nil
}

// Create /creatures/create check [ false false false ] access [ true false false false POST ]
func (a *Actions) Create(state state.IState, input inputs_creatures.CreateInput) (any, error) {
	trx := state.Trx()
	creatureType := input.Type
	username := input.Username + "@" + state.Source()
	chainId := "main"
	subchainId := "main"
	ownerId := "free"
	if input.ChainId != nil && *input.ChainId != "" {
		chainId = *input.ChainId
	}
	if input.SubchainId != nil && *input.SubchainId != "" {
		subchainId = *input.SubchainId
	}
	if input.OwnerId != nil && *input.OwnerId != "" {
		ownerId = *input.OwnerId
	}
	if creatureType == "human" {
		chainId = "main"
		subchainId = "main"
		ownerId = "free"
	} else if ownerId == "free" {
		ownerId = state.Info().UserId()
	}
	if trx.HasIndex("Creature", "username", "id", username) {
		return nil, errors.New("creature username already exists")
	}
	balance := int64(0)
	if creatureType == "human" {
		balance = 1000000000000000
	}
	creature := model.Creature{
		Id:         a.App.Tools().Storage().GenId(trx, input.Origin()),
		TypeName:   creatureType,
		Username:   username,
		PublicKey:  input.PublicKey,
		ChainId:    chainId,
		SubchainId: subchainId,
		OwnerId:    ownerId,
		Balance:    balance,
	}
	creature.Push(trx)
	// Mirror to the User table so older code paths (security, signaler, guards)
	// that read obj::User::{id}::{col} continue to work.
	model.User{
		Id:        creature.Id,
		Typ:       creature.TypeName,
		Username:  creature.Username,
		PublicKey: creature.PublicKey,
		Balance:   creature.Balance,
	}.Push(trx)
	// Machine-type creatures double as "machines" in the program/Deploy API,
	// which keys off the Machine table. Mirror the row so /programs/create and
	// /programs/deploy can find this creature by AppId.
	if creatureType == "machine" {
		model.Machine{
			Id:       creature.Id,
			ChainId:  creature.ChainId,
			OwnerId:  creature.OwnerId,
			Username: creature.Username,
		}.Push(trx)
	}
	session := model.Session{Id: a.App.Tools().Storage().GenId(trx, input.Origin()), UserId: creature.Id}
	session.Push(trx)
	trx.PutJson("CreatMeta::"+creature.Id, "metadata", input.Metadata, false)
	trx.PutJson("UserMeta::"+creature.Id, "metadata", input.Metadata, false)
	if creatureType != "human" {
		trx.PutLink("ownerof::"+ownerId+"::"+creature.Id, "true")
	}
	return map[string]any{"creature": creature, "session": session}, nil
}

// Get /creatures/get check [ true false false ] access [ true false false false GET ]
func (a *Actions) Get(state state.IState, input inputsusers.GetInput) (any, error) {
	trx := state.Trx()
	if trx.HasObj("Creature", input.UserId) {
		return map[string]any{"creature": model.Creature{Id: input.UserId}.Pull(trx)}, nil
	}
	return nil, errors.New("creature not found")
}

// List /creatures/list check [ true false false ] access [ true false false false GET ]
func (a *Actions) List(state state.IState, input inputsusers.ListInput) (any, error) {
	creatures, err := model.Creature{}.All(state.Trx(), input.Offset, input.Count)
	if err != nil {
		return nil, err
	}
	return map[string]any{"creatures": creatures}, nil
}

// Transfer /creatures/transfer check [ true false false ] access [ true false false false POST ]
func (a *Actions) Transfer(state state.IState, input inputsusers.TransferInput) (any, error) {
	trx := state.Trx()
	from := model.Creature{Id: state.Info().UserId()}.Pull(trx)
	if from.Id == "" {
		return nil, errors.New("sender creature not found")
	}
	if from.Balance < input.Amount {
		return nil, errors.New("your balance is not enough")
	}
	toId := trx.GetIndex("Creature", "username", "id", input.ToUsername)
	if toId == "" {
		return nil, errors.New("target creature not found")
	}
	to := model.Creature{Id: toId}.Pull(trx)
	if to.Id == "" {
		return nil, errors.New("target creature not found")
	}
	from.Balance -= input.Amount
	to.Balance += input.Amount
	from.Push(trx)
	to.Push(trx)
	return map[string]any{}, nil
}

// Signal /creatures/signal check [ true false false ] access [ true false false false POST ]
func (a *Actions) Signal(state state.IState, input inputs_creatures.SignalInput) (any, error) {
	trx := state.Trx()
	senderCreature := model.Creature{Id: state.Info().UserId()}.Pull(trx)
	sender := model.User{Id: senderCreature.Id, Typ: senderCreature.TypeName, Username: senderCreature.Username, PublicKey: senderCreature.PublicKey}
	storeId := state.Info().StoreId()
	if input.Type == "all" {
		if storeId == "" {
			return nil, errors.New("storeId is required for broadcast")
		}
		if trx.GetLink("onaccess::"+storeId+"::"+state.Info().UserId()) != "true" {
			return nil, errors.New("access denied")
		}
		packet := updates_stores.Send{Action: "broadcast", User: sender, Data: input.Data, IsTemp: input.Temp}
		future.Async(func() {
			a.App.Tools().Signaler().SignalGroup("creatures/signal", storeId, packet, true, []string{state.Info().UserId()})
		}, false)
		return map[string]any{"passed": true}, nil
	}
	if input.Type != "pvp" {
		return nil, errors.New("unknown signal type")
	}
	if input.CreatureId == "" {
		return nil, errors.New("creatureId is required for pvp")
	}
	// ProgramId targets a specific program within a machine creature;
	// falls back to CreatureId for direct user/program targets.
	targetId := input.CreatureId
	if input.ProgramId != "" {
		targetId = input.ProgramId
	}
	packet := updates_stores.Send{Action: "single", User: sender, Data: input.Data, IsTemp: input.Temp, EntityId: input.EntityId}
	future.Async(func() {
		a.App.Tools().Signaler().SignalUser("creatures/signal", targetId, packet, true)
	}, false)
	return map[string]any{"passed": true}, nil
}

// Authenticate /creatures/authenticate check [ true false false ] access [ true false false false POST ]
func (a *Actions) Authenticate(state state.IState, _ inputsusers.AuthenticateInput) (any, error) {
	_, res, _ := a.App.Actor().FetchAction("/creatures/get").Act(mainstate.NewState(base.NewInfo("", ""), state.Trx()), inputsusers.GetInput{UserId: state.Info().UserId()})
	// /creatures/get returns map[string]any{"creature": Creature}. Project it
	// onto the User-shaped Authenticate response that older clients expect.
	resMap, _ := res.(map[string]any)
	creature, _ := resMap["creature"].(model.Creature)
	userMap := map[string]any{
		"id":        creature.Id,
		"type":      creature.TypeName,
		"username":  creature.Username,
		"publicKey": creature.PublicKey,
		"balance":   creature.Balance,
	}
	return outputsusers.AuthenticateOutput{Authenticated: true, User: userMap}, nil
}

// Mint /creatures/mint check [ true false false ] access [ true false false false POST ]
func (a *Actions) Mint(state state.IState, input inputsusers.MintInput) (any, error) {
	if state.Info().UserId() != "1@global" {
		return nil, errors.New("access denied")
	}
	username := model.User{Id: state.Trx().GetLink("UserEmailToId::" + input.ToUserEmail)}.Pull(state.Trx()).Username
	toUserId := state.Trx().GetIndex("User", "username", "id", username)
	if toUserId == "" {
		return nil, errors.New("target user not found")
	}
	toUser := model.User{Id: toUserId}.Pull(state.Trx())
	toUser.Balance += input.Amount
	toUser.Push(state.Trx())
	model.Creature{Id: toUser.Id, TypeName: toUser.Typ, Username: toUser.Username, PublicKey: toUser.PublicKey, ChainId: "main", SubchainId: "main", OwnerId: "free", Balance: toUser.Balance}.Push(state.Trx())
	return map[string]any{}, nil
}

// CheckSign /creatures/checkSign check [ true false false ] access [ true false false false POST ]
func (a *Actions) CheckSign(state state.IState, input inputsusers.CheckSignInput) (any, error) {
	if state.Info().UserId() != "1@global" {
		return nil, errors.New("access denied")
	}
	data, err := base64.StdEncoding.DecodeString(input.Payload)
	if err != nil {
		log.Println(err)
		return map[string]any{"valid": false}, nil
	}
	if success, _, _ := a.App.Tools().Security().AuthWithSignature(input.UserId, data, input.Signature); success {
		email := state.Trx().GetLink("UserIdToEmail::" + input.UserId)
		return map[string]any{"valid": true, "email": email}, nil
	}
	return map[string]any{"valid": false}, nil
}

// LockToken /creatures/lockToken check [ true false false ] access [ true false false false POST ]
func (a *Actions) LockToken(state state.IState, input inputsusers.LockTokenInput) (any, error) {
	user := model.User{Id: state.Info().UserId()}.Pull(state.Trx())

	steps := make([]lockedTokenStep, 0, len(input.Steps))
	if len(input.Steps) > 0 {
		for i, step := range input.Steps {
			if step.Amount <= 0 {
				return nil, fmt.Errorf("step %d amount must be greater than zero", i)
			}
			if step.UnlockAt <= 0 {
				return nil, fmt.Errorf("step %d unlockAt must be a unix timestamp in milliseconds", i)
			}
			steps = append(steps, lockedTokenStep{Amount: step.Amount, UnlockAt: step.UnlockAt})
		}
	} else {
		if input.Amount <= 0 {
			return nil, errors.New("amount must be greater than zero")
		}
		if input.UnlockAt <= 0 {
			return nil, errors.New("unlockAt must be a unix timestamp in milliseconds")
		}
		steps = append(steps, lockedTokenStep{Amount: input.Amount, UnlockAt: input.UnlockAt})
	}

	totalAmount := int64(0)
	for _, step := range steps {
		totalAmount += step.Amount
	}
	if user.Balance < totalAmount {
		return nil, errors.New("your balance is not enough")
	}
	lockId := crypto.SecureUniqueString()
	if input.Type == "pay" {
		if !state.Trx().HasObj("User", input.Target) {
			return nil, errors.New("target user not acceptable")
		}
		user.Balance -= totalAmount
		user.Push(state.Trx())
		state.Trx().PutJson("Json::User::"+state.Info().UserId(), "lockedTokens."+lockId, map[string]any{
			"type":            "pay",
			"amount":          totalAmount,
			"remainingAmount": totalAmount,
			"userId":          input.Target,
			"steps":           steps,
		}, true)
	} else {
		return nil, errors.New("unknown lock type")
	}
	return map[string]any{"tokenId": lockId}, nil
}

// ConsumeLock /creatures/consumeLock check [ true false false ] access [ true false false false POST ]
func (a *Actions) ConsumeLock(state state.IState, input inputsusers.ConsumeLockInput) (any, error) {
	receiver := model.User{Id: state.Info().UserId()}.Pull(state.Trx())
	if input.Type == "pay" {
		if !state.Trx().HasObj("User", input.UserId) {
			return nil, errors.New("payer user not found")
		}
		sender := model.User{Id: input.UserId}.Pull(state.Trx())
		if payment, err := state.Trx().GetJson("Json::User::"+sender.Id, "lockedTokens."+input.LockId); err == nil {
			stepsRaw, ok := payment["steps"].([]any)
			if !ok || len(stepsRaw) == 0 {
				return nil, errors.New("lock does not include steps")
			}
			stepIndex := -1
			if input.Step != nil {
				stepIndex = *input.Step
			}
			now := time.Now().UnixMilli()
			parsedSteps := make([]map[string]any, len(stepsRaw))
			parsedAmounts := make([]int64, len(stepsRaw))
			parsedUnlocks := make([]int64, len(stepsRaw))
			for i, rawStep := range stepsRaw {
				stepMap, ok := rawStep.(map[string]any)
				if !ok {
					return nil, errors.New("invalid lock step")
				}
				parsedSteps[i] = stepMap
				stepAmount, ok := asInt64(stepMap["amount"])
				if !ok || stepAmount <= 0 {
					return nil, errors.New("invalid lock step amount")
				}
				unlockAt, ok := asInt64(stepMap["unlockAt"])
				if !ok || unlockAt <= 0 {
					return nil, errors.New("invalid lock step unlockAt")
				}
				parsedAmounts[i] = stepAmount
				parsedUnlocks[i] = unlockAt
				if stepIndex == -1 {
					consumed, _ := stepMap["consumed"].(bool)
					if !consumed && now >= unlockAt && stepAmount == input.Amount {
						stepIndex = i
					}
				}
			}
			if stepIndex < 0 || stepIndex >= len(parsedSteps) {
				return nil, errors.New("lock step not found")
			}
			selectedStep := parsedSteps[stepIndex]
			selectedAmount := parsedAmounts[stepIndex]
			selectedUnlockAt := parsedUnlocks[stepIndex]
			if now < selectedUnlockAt {
				return nil, errors.New("lock step is not consumable yet")
			}
			if consumed, _ := selectedStep["consumed"].(bool); consumed {
				return nil, errors.New("lock step already consumed")
			}
			if input.Amount != selectedAmount {
				return nil, errors.New("amount of payment not matched")
			}
			signPayload := []byte(fmt.Sprintf("%s:%d:%d:%d:%s", input.LockId, stepIndex, selectedUnlockAt, selectedAmount, receiver.Id))
			if success, _, _ := a.App.Tools().Security().AuthWithSignature(input.UserId, signPayload, input.Signature); success {
				if typ, ok := payment["type"].(string); ok && (typ == "pay") {
					if target, ok := payment["userId"].(string); ok && (target == receiver.Id) {
						selectedStep["consumed"] = true
						selectedStep["consumedAt"] = now
						receiver.Balance += input.Amount
						receiver.Push(state.Trx())
						remainingAmount := int64(0)
						for i := range parsedSteps {
							consumed, _ := parsedSteps[i]["consumed"].(bool)
							if !consumed {
								remainingAmount += parsedAmounts[i]
							}
						}
						if remainingAmount == 0 {
							state.Trx().DelJson("Json::User::"+sender.Id, "lockedTokens."+input.LockId)
						} else {
							totalAmount, ok := asInt64(payment["amount"])
							if !ok {
								return nil, errors.New("invalid lock total amount")
							}
							payment["steps"] = parsedSteps
							payment["remainingAmount"] = remainingAmount
							payment["consumedAmount"] = totalAmount - remainingAmount
							state.Trx().PutJson("Json::User::"+sender.Id, "lockedTokens."+input.LockId, payment, true)
						}
						return map[string]any{"success": true, "step": stepIndex, "remainingAmount": remainingAmount}, nil
					}
					return nil, errors.New("you are not target")
				}
				return nil, errors.New("type is not payment")
			}
			return nil, errors.New("signature not verified")
		}
		return nil, errors.New("lock not found")
	}
	return nil, errors.New("unknown lock type")
}

// Login /creatures/login check [ false false false ] access [ true false false false POST ]
func (a *Actions) Login(state state.IState, input inputsusers.LoginInput) (any, error) {
	ctx := a.OauthCtx
	var email string
	if a.firebaseApp == nil {
		// DEV mode: no Firebase configured. Treat emailToken as raw email
		// (or fall back to username@dev.local if blank). This makes the node
		// usable for local development without provisioning Firebase Auth.
		email = strings.TrimSpace(input.EmailToken)
		if email == "" || !strings.Contains(email, "@") {
			email = input.Username + "@dev.local"
		}
		log.Println("[DEV] firebase disabled; accepting login for email:", email)
	} else {
		client, err := a.firebaseApp.Auth(ctx)
		if err != nil {
			log.Println(err)
			e := errors.New("error getting Auth client")
			log.Println(e)
			return nil, e
		}
		token, err := client.VerifyIDToken(ctx, input.EmailToken)
		if err != nil {
			log.Println(err)
			e := errors.New("invalid ID token")
			log.Println(e)
			return nil, e
		}
		var ok bool
		email, ok = token.Claims["email"].(string)
		if !ok {
			e := errors.New("email claim not found or invalid")
			log.Println(e)
			return nil, e
		}
	}

	trx := state.Trx()
	userId := trx.GetLink("UserEmailToId::" + email)
	log.Println("fetching email:", "["+email+"]", "["+userId+"]")
	if userId != "" {
		user := model.User{Id: userId}.Pull(trx)
		session := model.Session{Id: trx.GetIndex("Session", "userId", "id", user.Id)}.Pull(trx)
		privKey := trx.GetLink("UserPrivateKey::" + user.Id)
		return outputsusers.LoginOutput{User: user, Session: session, PrivateKey: privKey}, nil
	}
	if !trx.HasIndex("User", "username", "id", input.Username+"@"+a.App.Id()) {
		priKeyRaw, pubKeyRaw := crypto.SecureKeyPairs("")
		priKey := string(priKeyRaw)
		pubKey := string(pubKeyRaw)
		req := inputs_creatures.CreateInput{
			Type:      "human",
			Username:  input.Username,
			PublicKey: pubKey,
			Metadata:  input.Metadata,
		}
		// Call /creatures/create's action directly on the current state.
		// Going through SecurelyAct would re-submit to the chain (origin =
		// "global") and deadlock: we're already inside the chain pipeline
		// processing /creatures/login, and chain processing is single-threaded.
		_, res, err2 := a.App.Actor().FetchAction("/creatures/create").Act(state, req)
		if err2 != nil {
			return nil, err2
		}
		resMap, ok := res.(map[string]any)
		if !ok {
			return nil, errors.New("unexpected /creatures/create response shape")
		}
		creature, _ := resMap["creature"].(model.Creature)
		session, _ := resMap["session"].(model.Session)
		userVal := model.User{Id: creature.Id, Typ: creature.TypeName, Username: creature.Username, PublicKey: creature.PublicKey, Balance: creature.Balance}
		trx.PutLink("UserPrivateKey::"+userVal.Id, priKey)
		trx.PutLink("UserEmailToId::"+email, userVal.Id)
		trx.PutLink("UserIdToEmail::"+userVal.Id, email)
		log.Println("saving email:", "["+email+"]", "["+userVal.Id+"]")
		return outputsusers.LoginOutput{User: userVal, Session: session, PrivateKey: priKey}, nil
	}
	return nil, errors.New("username already exist")
}

// Delete /creatures/delete check [ true false false ] access [ true false false false POST ]
func (a *Actions) Delete(state state.IState, input inputsusers.DeleteInput) (any, error) {
	if input.UserId != state.Info().UserId() && state.Info().UserId() != "1@global" {
		return nil, errors.New("access denied")
	}
	user := model.User{Id: input.UserId}.Pull(state.Trx())
	if user.Id == "" {
		return nil, errors.New("user not found")
	}
	user.Delete(state.Trx())
	state.Trx().DelJson("UserMeta::"+input.UserId, "metadata")
	storeList, _ := model.Store{}.List(state.Trx(), "hasaccess::"+input.UserId+"::", false, map[string]string{}, map[string][]string{})
	for _, store := range storeList {
		state.Trx().DelKey("link::onaccess::" + store.Id + "::" + input.UserId)
	}
	createdStoreList, _ := model.Store{}.List(state.Trx(), "creatorof::"+input.UserId+"::", false, map[string]string{}, map[string][]string{})
	for _, store := range createdStoreList {
		store.Delete(state.Trx())
	}
	state.Trx().DelKey("link::UserIdToEmail::" + input.UserId)
	state.Trx().DelKey("link::UserEmailToId::" + state.Trx().GetLink("UserIdToEmail::"+input.UserId))
	state.Trx().DelKey("link::UserPrivateKey::" + input.UserId)
	return map[string]any{}, nil
}

// Update /creatures/update check [ true false false ] access [ true false false false POST ]
func (a *Actions) Update(state state.IState, input inputsusers.UpdateInput) (any, error) {
	if input.UserId != state.Info().UserId() && state.Info().UserId() != "1@global" {
		return nil, errors.New("access denied")
	}
	user := model.User{Id: input.UserId}.Pull(state.Trx())
	if user.Id == "" {
		return nil, errors.New("user not found")
	}
	if input.PublicKey != nil {
		user.PublicKey = *input.PublicKey
	}
	if input.Type != nil {
		user.Typ = *input.Type
	}
	if input.Username != nil {
		baseUsername := strings.Split(user.Username, "@")[0]
		if *input.Username != baseUsername {
			nextUsername := *input.Username + "@" + state.Source()
			if state.Trx().HasIndex("User", "username", "id", nextUsername) {
				return nil, errors.New("username already exists")
			}
			state.Trx().DelKey("index::User::username::id::" + user.Username)
			user.Username = nextUsername
		}
	}
	user.Push(state.Trx())
	model.Creature{Id: user.Id, TypeName: user.Typ, Username: user.Username, PublicKey: user.PublicKey, ChainId: "main", SubchainId: "main", OwnerId: "free", Balance: user.Balance}.Push(state.Trx())
	return map[string]any{}, nil
}

// Meta /creatures/meta check [ true false false ] access [ true false false false GET ]
func (a *Actions) Meta(state state.IState, input inputsusers.MetaInput) (any, error) {
	user := model.User{Id: input.UserId}.Pull(state.Trx())
	if user.Id == "" {
		return nil, errors.New("user not found")
	}
	return state.Trx().GetJson("UserMeta::"+input.UserId, "metadata")
}

// GetByUsername /creatures/getByUsername check [ true false false ] access [ true false false false GET ]
func (a *Actions) GetByUsername(state state.IState, input inputsusers.GetByUsernameInput) (any, error) {
	userId := state.Trx().GetIndex("User", "username", "id", input.Username)
	if userId == "" {
		return nil, errors.New("user not found")
	}
	result := model.User{Id: userId}.Pull(state.Trx())
	m, _ := trx.ObjectToMap(result)
	if ex, ok := a.modelExtender["user"]; ok {
		for name, field := range ex {
			if field.GetValue != nil {
				m[name], _ = field.GetValue(state, m)
			}
		}
	}
	return outputsusers.GetOutput{User: m}, nil
}

// Find /creatures/find check [ true false false ] access [ true false false false GET ]
func (a *Actions) Find(state state.IState, input inputsusers.FindInput) (any, error) {
	users, _ := model.User{}.Search(state.Trx(), 0, 1, "username", input.Username, map[string]string{})
	if len(users) == 0 {
		return nil, errors.New("user not found")
	}
	result := users[0]
	m, _ := trx.ObjectToMap(result)
	if ex, ok := a.modelExtender["user"]; ok {
		for name, field := range ex {
			if field.GetValue != nil {
				m[name], _ = field.GetValue(state, m)
			}
		}
	}
	return outputsusers.GetOutput{User: m}, nil
}
