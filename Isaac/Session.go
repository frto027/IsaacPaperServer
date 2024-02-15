/*
	IsaacPaperServer server program
	Copyright (C) 2024  frto027

	This program is free software: you can redistribute it and/or modify
	it under the terms of the GNU Affero General Public License as
	published by the Free Software Foundation, either version 3 of the
	License, or (at your option) any later version.

	This program is distributed in the hope that it will be useful,
	but WITHOUT ANY WARRANTY; without even the implied warranty of
	MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
	GNU Affero General Public License for more details.

	You should have received a copy of the GNU Affero General Public License
	along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

package Isaac

import (
	"IsaacPaperServer/0xf7.top/IsaacPaperServer/Isaacpb"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
)

type SteamID uint64

type SessionData struct {
	closed        bool
	hasLogin      bool
	conn          net.Conn
	steamId       SteamID
	name          string
	currentLobby  LobbyID
	connSendMutex sync.Mutex
	langId        Isaacpb.RequestLogin_Lang

	lastWaitToken string
}

func (s *SessionData) Create(conn net.Conn) {
	s.conn = conn
	s.closed = false
	s.hasLogin = false
	s.currentLobby = 0
	s.connSendMutex = sync.Mutex{}
}
func (s *SessionData) IsAlive() bool {
	if s.closed {
		return false
	}
	return true
}

func (L *LobbyData) SendPackageToAllUsers(messageType Isaacpb.ResponseHeader_ResponseMessageType, holdValue int32, m proto.Message, except SteamID) {
	for _, userSteamId := range L.users {
		if userSteamId == except {
			continue
		}
		sessionsMutex.Lock()
		session, ok := sessions[userSteamId]
		sessionsMutex.Unlock()
		if !ok {
			continue // impossible...?
		}
		session.SendPackage(messageType, holdValue, m)
	}
}

func (L *LobbyData) SendUserInfoToAllUsers() {
	for _, userSteamId := range L.users {
		if userSteamId == SteamID(0) {
			continue
		}
		sessionsMutex.Lock()
		session, ok := sessions[userSteamId]
		sessionsMutex.Unlock()
		if !ok {
			continue // impossible...?
		}
		session.SendUserInfos(L)
	}
}

func (s *SessionData) SendUserInfo(id SteamID) {
	sessionsMutex.Lock()
	ss, ok := sessions[id]
	sessionsMutex.Unlock()
	if !ok {
		return
	}
	s.SendPackage(Isaacpb.ResponseHeader_UpdateUserInfo, 0, &Isaacpb.ResponseUpdateUserInfo{
		UserId: uint64(id),
		Name:   ss.name,
	})
}

func (s *SessionData) SendUserInfos(L *LobbyData) {
	for _, u := range L.users {
		s.SendUserInfo(u)
	}
}

func (s *SessionData) JoinLobby(id LobbyID) bool {
	lobbiesMutex.Lock()
	L, ok := lobbies[id]
	lobbiesMutex.Unlock()
	if !ok {
		return false
	}
	_, err := L.AddUser(s.steamId)
	if err != nil {
		return false
	}

	s.currentLobby = id
	return true
}
func (s *SessionData) LeaveLobby() {
	if s.currentLobby == 0 {
		return
	}

	lobbiesMutex.Lock()
	lobby, ok := lobbies[s.currentLobby]
	lobbiesMutex.Unlock()

	if ok {
		lobby.lobbyMutex.Lock()
		lobby.RemoveUser(s.steamId)
		lobby.lobbyMutex.Unlock()
		//TODO: send leave user package to others
		if lobby.UserCount() == 0 {
			lobbiesMutex.Lock()
			delete(lobbies, s.currentLobby)
			lobbiesMutex.Unlock()
			log.Print("lobby ", lobby.id, " is empty, so remove it.")
		} else {
			lobby.SendPackageToAllUsers(Isaacpb.ResponseHeader_LobbyChatUpdate, 0, &Isaacpb.ResponseLobbyChatUpdate{
				SteamIdLobby:               uint64(lobby.id),
				SteamIdUserChanged:         uint64(s.steamId),
				SteamIdMakingChange:        uint64(s.steamId),
				SteamIdMakingChangeIsLobby: false,
				SteamIdUserChangedIsLobby:  false,
				ChatMemberStateChange:      Isaacpb.ResponseLobbyChatUpdate_Left,
			}, s.steamId)
		}
	}

	s.currentLobby = 0

	if len(s.lastWaitToken) > 0 {
		waitingClientsMutex.Lock()
		if _, ok = waitingClients[s.lastWaitToken]; ok {
			delete(waitingClients, s.lastWaitToken)
		}
		waitingClientsMutex.Unlock()
	}
}

func (s *SessionData) SendLobbyDataUpdate(steamIdMember SteamID, steamIdLobby LobbyID, onlyLobby bool) bool {
	return s.SendPackage(Isaacpb.ResponseHeader_LobbyDataUpdate, 0, &Isaacpb.ResponseLobbyDataUpdate{
		SteamIdLobby:  uint64(steamIdLobby),
		SteamIdMember: uint64(steamIdMember),
		OnlyLobbyId:   onlyLobby,
	})
}
func (s *SessionData) SendPackage(messageType Isaacpb.ResponseHeader_ResponseMessageType, holdValue int32, m proto.Message) bool {
	s.connSendMutex.Lock()
	defer s.connSendMutex.Unlock()
	return s.SendPackageNoLock(messageType, holdValue, m)
}
func (s *SessionData) SendPackageNoLock(messageType Isaacpb.ResponseHeader_ResponseMessageType, holdValue int32, m proto.Message) bool {
	bts, err := proto.Marshal(m)
	if err != nil {
		log.Print(err)
		return false
	}

	header := Isaacpb.ResponseHeader{}
	header.Type = messageType
	header.HoldValue = holdValue
	header.Length = int32(len(bts))
	if len(bts) >= 4096 {
		log.Print("Message is too long")
		return false
	}

	hbts, err := proto.Marshal(&header)
	if err != nil {
		log.Print(err)
		return false
	}

	hbtsLen := len(hbts)
	n_bts := make([]byte, 4)
	binary.BigEndian.PutUint32(n_bts, uint32(hbtsLen))
	sent := 0
	for sent < 4 {
		o, err := s.conn.Write(n_bts[sent:4])
		if err != nil {
			log.Print(err)
			return false
		}
		if o < 0 {
			log.Print("Failed send")
			return false
		}
		sent += o
	}
	sent = 0
	for sent < hbtsLen {
		o, err := s.conn.Write(hbts[sent:hbtsLen])
		if err != nil {
			log.Print(err)
			return false
		}
		if o < 0 {
			log.Print("Failed send")
			return false
		}
		sent += o
	}
	sent = 0
	for sent < len(bts) {
		o, err := s.conn.Write(bts[sent:])
		if err != nil {
			log.Print(err)
			return false
		}
		if o < 0 {
			log.Print("Failed send")
			return false
		}
		sent += o
	}
	return true
}

func (s *SessionData) HandlePackage(header *Isaacpb.RequestHeader, body []byte) error {
	if !s.hasLogin {
		if header.Type == Isaacpb.RequestHeader_AdminLogin {
			log.Print("An admin from ", s.conn.RemoteAddr(), " is connected.")
			HandleAdminSession(s)
			return errors.New("admin session has been closed")
		}
		if header.Type != Isaacpb.RequestHeader_Login {
			return errors.New("the first package is not login")
		}
	}
	switch header.Type {
	case Isaacpb.RequestHeader_Time:
		log.Print("user ", s.name, " request server time")
		if !s.SendPackage(
			Isaacpb.ResponseHeader_Time, 0,
			&Isaacpb.ResponseTime{Timestamp: uint32(time.Now().Unix())}) {
			return errors.New("failed to send server time package")
		}
	case Isaacpb.RequestHeader_Login:
		msg := Isaacpb.RequestLogin{}
		if err := proto.Unmarshal(body, &msg); err != nil {
			log.Print(err)
			return errors.New("failed to parse login package")
		}

		if s.steamId != 0 {
			return errors.New("login twice is not allowed")
		}

		userBlocked := false
		blockReason := ""
		userAccessMutex.Lock()
		switch userAccessMode {
		case USER_MODE_PRIVATE:
			user, ok := userAccess[SteamID(msg.SteamID)]
			if !ok {
				userBlocked = true
				blockReason = "服务器为白名单模式，您不在列表中，请联系服务器管理员"
			}
			if user.access != USER_ACCESS_ALLOW {
				userBlocked = true
				blockReason = *user.blockReason
			}
		case USER_MODE_PUBLIC:
			user, ok := userAccess[SteamID(msg.SteamID)]
			if ok && user.access == USER_ACCESS_DENY {
				userBlocked = true
				blockReason = *user.blockReason
			}
		}
		userAccessMutex.Unlock()

		s.langId = msg.GetLangCode()

		if userBlocked {
			caption := "服务器已阻止您的连接"
			s.SendPackage(Isaacpb.ResponseHeader_ServerPublicMessage, 0, &Isaacpb.ResponseServerPublicMessage{
				Type:    Isaacpb.ResponseServerPublicMessage_DisplayStringAndExit,
				Str:     &blockReason,
				Caption: &caption,
			})
			time.Sleep(time.Second * 3)
			return errors.New(fmt.Sprint("user ", msg.Name, "(", msg.SteamID, ") is blocked, because ", blockReason))
		}

		s.name = msg.Name
		s.steamId = SteamID(msg.SteamID)

		s.hasLogin = true

		if msg.ProtocolVer != PROTOCOL_VER {
			log.Print("user ", msg.Name, "(", msg.SteamID, ") ", " has mismatched proto_ver:", msg.ProtocolVer, " block it")
			caption := "PaperCup version mismatch!"
			hint := fmt.Sprint("Your version(", msg.ProtocolVer, ") is not match the server version(", PROTOCOL_VER, ")")
			s.SendPackage(Isaacpb.ResponseHeader_ServerPublicMessage, 0, &Isaacpb.ResponseServerPublicMessage{
				Type:    Isaacpb.ResponseServerPublicMessage_DisplayStringAndExit,
				Caption: &caption,
				Str:     &hint,
			})
			return errors.New(fmt.Sprint("server protocol mismatch, client is ", msg.ProtocolVer))
		}

		sessionsMutex.Lock()
		sessions[s.steamId] = s
		sessionsMutex.Unlock()

		if true {
			caption := "Welcome to Isaac Paper Phone"
			hint := "Welcome to this server.\n  - a friendly message from admin."

			if s.langId == Isaacpb.RequestLogin_ZH {
				caption = "欢迎来到“以撒的纸电话”"
				hint = "欢迎来到这个服务器。\n本服务器程序由Frto027制作。\n您的一切行为均对管理员可见，请勿交流敏感内容，否则会被管理员封禁。"
			}
			s.SendPackage(Isaacpb.ResponseHeader_ServerPublicMessage, 0, &Isaacpb.ResponseServerPublicMessage{
				Type:    Isaacpb.ResponseServerPublicMessage_DisplayStringAndContinue,
				Caption: &caption,
				Str:     &hint,
			})
		}

		DefaultLobbyNamesMutex.Lock()
		if DefaultLobbyNames != nil {
			s.SendPackage(Isaacpb.ResponseHeader_UpdateCreateRoomNameLists, 0, &Isaacpb.ResponseCreateRoomNameLists{
				Names: *DefaultLobbyNames,
			})
		}
		DefaultLobbyNamesMutex.Unlock()

		DefaultFastChatMessagesMutex.Lock()
		if DefaultFastChatMessages != nil {
			s.SendPackage(Isaacpb.ResponseHeader_UpdateLogConsoleChatFastMessages, 0, &Isaacpb.ResponseLogConsoleChatFastMessage{
				Msgs: *DefaultFastChatMessages,
			})
		}
		DefaultFastChatMessagesMutex.Unlock()

		log.Print("user ", s.name, "(", s.steamId, ") is login!")
		log.Printf("user %s client crc value is %08x", s.name, msg.GetGameImageCrc())
	case Isaacpb.RequestHeader_LobbyList:
		msg := Isaacpb.RequestLobbyList{}
		if err := proto.Unmarshal(body, &msg); err != nil {
			log.Print(err)
			return errors.New("failed to parse lobby list package")
		}

		log.Print("user ", s.name, " request lobby list")

		r := Isaacpb.ResponseLobbyList{}

		lobbiesMutex.Lock()
		r.Lobbies = make([]*Isaacpb.LobbyInfo, len(lobbies))
		idx := 0
		for _, data := range lobbies {
			r.Lobbies[idx] = data.ToProtobufLobbyInfo()
			idx++
		}
		lobbiesMutex.Unlock()

		if !s.SendPackage(Isaacpb.ResponseHeader_LobbyList, header.HoldValue,
			&r) {
			return errors.New("failed to send lobby list package")
		}
	case Isaacpb.RequestHeader_LobbyCreate:
		msg := Isaacpb.RequestLobbyCreate{}
		if err := proto.Unmarshal(body, &msg); err != nil {
			log.Print(err)
			return errors.New("failed to parse lobby create info package")
		}

		lobby := LobbyData{}
		lobby.Create()

		lobby.name = filterStr(msg.Name)
		lobby.password = msg.Password

		_, _ = lobby.AddUser(s.steamId)
		lobby.owner = s.steamId
		info := lobby.ToProtobufLobbyInfo()
		if lobby.password != nil {
			//the user that create the lobby knows the password
			info.Password = lobby.password
		}
		if !s.SendPackage(Isaacpb.ResponseHeader_LobbyCreated, header.HoldValue,
			&Isaacpb.ResponseLobbyCreated{
				LobbyId: uint64(lobby.id),
				Info:    info,
			}) {
			return errors.New("failed to send lobby created package")
		}

		isLocked := "locked "
		if lobby.password == nil {
			isLocked = ""
		}
		log.Print(isLocked, "lobby ", lobby.name, "(", lobby.id, ") is created by steam user ", s.name, "(", s.steamId, ")")
		lobbiesMutex.Lock()
		lobbies[lobby.id] = &lobby
		lobbiesMutex.Unlock()

		if !s.SendLobbyDataUpdate(s.steamId, lobby.id, true) {
			return errors.New("failed to send lobby data update package")
		}
	case Isaacpb.RequestHeader_SetLobbyData:
		msg := Isaacpb.RequestSetLobbyData{}
		if err := proto.Unmarshal(body, &msg); err != nil {
			log.Print(err)
			return errors.New("failed to parse SetLobbyData package")
		}
		log.Print("user ", s.name, "(", s.steamId, ") wants set lobby ", msg.LobbyID, " [", msg.PchKey, "]=", msg.PchValue)
		lobbiesMutex.Lock()
		L, ok := lobbies[LobbyID(msg.LobbyID)]
		if !ok {
			lobbiesMutex.Unlock()
			return errors.New(fmt.Sprint("SetLobbyData: lobby ID not found:", msg.LobbyID))
		}

		if L.UserPosition(s.steamId) == -1 {
			lobbiesMutex.Unlock()
			return errors.New(fmt.Sprint("SetLobbyData: user is not in the lobby"))
		}
		L.data[msg.PchKey] = msg.PchValue
		lobbiesMutex.Unlock()

		//FIXME: lock?
		L.SendPackageToAllUsers(Isaacpb.ResponseHeader_LobbyDataUpdate, 0, &Isaacpb.ResponseLobbyDataUpdate{
			SteamIdLobby:  uint64(L.id),
			SteamIdMember: uint64(s.steamId),
			OnlyLobbyId:   true,
			Datas:         []*Isaacpb.LobbyDataUpdateItem{&Isaacpb.LobbyDataUpdateItem{K: msg.PchKey, V: msg.PchValue}},
		}, 0)
	case Isaacpb.RequestHeader_SetLobbyMemberData:
		msg := Isaacpb.RequestSetLobbyMemberData{}
		if err := proto.Unmarshal(body, &msg); err != nil {
			log.Print(err)
			return errors.New("failed to parse SetLobbyMemberData package")
		}
		log.Print("user ", s.name, "(", s.steamId, ") wants set lobby ", msg.LobbyID, " member data [", msg.PchKey, "]=", msg.PchValue)
		lobbiesMutex.Lock()
		L, ok := lobbies[LobbyID(msg.LobbyID)]
		lobbiesMutex.Unlock()
		if !ok {
			log.Print("lobby not found")
			return errors.New("lobby not found")
		}
		pos := L.UserPosition(s.steamId)
		if pos == -1 {
			log.Print("invalid user")
			return errors.New("invalid user")
		}
		L.memberData[pos][msg.PchKey] = msg.PchValue
		//FIXME: lock?
		L.SendPackageToAllUsers(Isaacpb.ResponseHeader_LobbyMemberDataUpdate, 0, &Isaacpb.ResponseLobbyDataUpdate{
			SteamIdLobby:  msg.LobbyID,
			SteamIdMember: uint64(s.steamId),
			OnlyLobbyId:   false,
			Datas:         []*Isaacpb.LobbyDataUpdateItem{&Isaacpb.LobbyDataUpdateItem{K: msg.PchKey, V: msg.PchValue}},
		}, s.steamId)
	case Isaacpb.RequestHeader_LobbyJoin:
		msg := Isaacpb.RequestJoinLobby{}
		if err := proto.Unmarshal(body, &msg); err != nil {
			log.Print(err)
			return errors.New("failed to parse JoinLobby package")
		}
		log.Print("user ", s.name, "(", s.steamId, ") wants join lobby ", msg.LobbyID)
		if s.currentLobby != LobbyID(0) {
			log.Print("the user is already in lobby ", s.currentLobby, ", we will let the user leave")
			return errors.New("user already in lobby")
		}

		resp := Isaacpb.ResponseLobbyJoin{}

		lobbiesMutex.Lock()
		lobby, ok := lobbies[LobbyID(msg.LobbyID)]
		lobbiesMutex.Unlock()

		if ok &&
			(lobby.password == nil || (msg.Password != nil && *(msg.Password) == *(lobby.password))) &&
			s.JoinLobby(LobbyID(msg.LobbyID)) {

			resp.LobbyId = uint32(msg.LobbyID)
			resp.Locked = false
			resp.ChatRoomEnterResponse = uint32(Isaacpb.ResponseLobbyJoin_Success)
			resp.ChatPermissions = 1
			resp.Info = lobby.ToProtobufLobbyInfoWithUserData()
		} else {
			resp.LobbyId = uint32(msg.LobbyID)
			resp.Locked = false
			resp.ChatRoomEnterResponse = uint32(Isaacpb.ResponseLobbyJoin_NotAllowed)
			resp.ChatPermissions = 1
			if ok && lobby.password != nil && (msg.Password == nil || *(msg.Password) != *(lobby.password)) {
				str := "密码错误"
				c := "房间加入失败"
				s.SendPackage(Isaacpb.ResponseHeader_ServerPublicMessage, 0, &Isaacpb.ResponseServerPublicMessage{
					Type:    Isaacpb.ResponseServerPublicMessage_DisplayStringAtLogConsole,
					Str:     &str,
					Caption: &c,
				})
			}
		}
		lobby.SendUserInfoToAllUsers()
		s.SendPackage(Isaacpb.ResponseHeader_LobbyJoin, header.HoldValue, &resp)
		lobby.SendPackageToAllUsers(Isaacpb.ResponseHeader_LobbyChatUpdate, 0, &Isaacpb.ResponseLobbyChatUpdate{
			SteamIdLobby:               uint64(lobby.id),
			SteamIdMakingChange:        0,
			SteamIdUserChanged:         uint64(s.steamId),
			SteamIdMakingChangeIsLobby: false,
			SteamIdUserChangedIsLobby:  false,
			ChatMemberStateChange:      Isaacpb.ResponseLobbyChatUpdate_Entered,
			LobbyInfo:                  lobby.ToProtobufLobbyInfoWithUserData(),
		}, s.steamId)
		/*
			lobby.SendPackageToAllUsers(Isaacpb.ResponseHeader_LobbyDataUpdate, 0, &Isaacpb.ResponseLobbyDataUpdate{
				SteamIdLobby:  uint64(lobby.id),
				SteamIdMember: uint64(s.steamId),
				OnlyLobbyId:   false,
			}, s.steamId)*/
	case Isaacpb.RequestHeader_SendP2PPackage:
		msg := Isaacpb.RequestSendP2PPackage{}
		if err := proto.Unmarshal(body, &msg); err != nil {
			log.Print(err)
			return errors.New("failed to parse SendP2PPackage package")
		}

		//TODO: check authority(same lobby)
		sessionsMutex.Lock()
		other, hasOtherSession := sessions[SteamID(msg.SteamIDRemote)]
		sessionsMutex.Unlock()

		if !hasOtherSession {
			log.Print("unknown target ", msg.SteamIDRemote)
		}

		read := uint32(0)
		blockSize := msg.FollowingDataSize
		// the limit
		if blockSize > 1024*1024 {
			blockSize = 1024 * 1024
		}

		buffer := make([]byte, blockSize)

		firstBlockHasSent := false

		currentBlockRead := uint32(0)
		currentBlockSize := min(blockSize, msg.FollowingDataSize-read)

		if currentBlockSize != msg.FollowingDataSize {
			log.Print("a splited block send")
		}

		for read < msg.FollowingDataSize {
			r, err := s.conn.Read(buffer[currentBlockRead:currentBlockSize])
			if err != nil || r < 0 {
				if firstBlockHasSent {
					needToSend := msg.FollowingDataSize - (read - currentBlockRead)
					sent := uint32(0)
					zeros := make([]byte, needToSend)
					otherSideHasError := false
					for sent < needToSend {
						r, err := other.conn.Write(zeros[sent:])
						if err != nil || r < 0 {
							//the other side is also error
							otherSideHasError = true
							break
						}
						sent += uint32(r)
					}
					if !otherSideHasError {
						b := []byte{0}
						_, _ = other.conn.Write(b)
					}
					other.connSendMutex.Unlock()
				}
				return errors.New("failed to read package")
			}
			currentBlockRead += uint32(r)
			read += uint32(r)
			if currentBlockRead == currentBlockSize {
				//send this block
				if hasOtherSession {
					if !firstBlockHasSent {
						firstBlockHasSent = true
						other.connSendMutex.Lock()
						other.SendPackageNoLock(Isaacpb.ResponseHeader_HasNewP2PPackage, 0, &Isaacpb.ResponseHasNewP2PPackage{
							SteamIDSource: uint64(s.steamId),
							DataSize:      msg.FollowingDataSize,
							Channel:       msg.Channel,
						})
					}

					sent := uint32(0)
					for sent < currentBlockSize {
						theSentByte, err := other.conn.Write(buffer[sent:currentBlockSize])
						if err != nil || theSentByte < 0 {
							hasOtherSession = false
							if firstBlockHasSent {
								other.connSendMutex.Unlock()
								firstBlockHasSent = false
							}
						}
						sent += uint32(theSentByte)
					}
				}
				// receive next block
				currentBlockRead = 0
				currentBlockSize = min(blockSize, msg.FollowingDataSize-read)
			}
		}
		if firstBlockHasSent {
			// send one more byte to inform if success
			b := []byte{1}
			_, _ = other.conn.Write(b)
			other.connSendMutex.Unlock()
			//log.Print("a buffer sent from ", s.name, " -> ", other.name, " (", msg.FollowingDataSize, ")")
		} else {
			//log.Print("a package was not sent")
		}
	case Isaacpb.RequestHeader_LobbyLeave:
		msg := Isaacpb.RequestLeaveLobby{}
		if err := proto.Unmarshal(body, &msg); err != nil {
			log.Print(err)
			return errors.New("failed to parse LeaveLobby package")
		}

		s.LeaveLobby()
	case Isaacpb.RequestHeader_GetServerUdpToken:
		//TODO remove old token
		nextToken := fmt.Sprint(s.steamId, rand.Int())

		if s.currentLobby == 0 {
			s.SendPackage(Isaacpb.ResponseHeader_ServerUdpToken, 0, &Isaacpb.ResponseServerUdpToken{Token: ""})
			return nil
		}

		lobbiesMutex.Lock()
		L, ok := lobbies[s.currentLobby]
		lobbiesMutex.Unlock()

		if !ok {
			s.SendPackage(Isaacpb.ResponseHeader_ServerUdpToken, 0, &Isaacpb.ResponseServerUdpToken{Token: ""})
			return nil
		}

		L.lobbyMutex.Lock()
		pos := L.UserPosition(s.steamId)
		L.lobbyMutex.Unlock()

		if pos == -1 {
			s.SendPackage(Isaacpb.ResponseHeader_ServerUdpToken, 0, &Isaacpb.ResponseServerUdpToken{Token: ""})
			return nil
		}

		waitingClientsMutex.Lock()
		if _, ok := waitingClients[s.lastWaitToken]; ok {
			delete(waitingClients, s.lastWaitToken)
		}
		waitingClients[nextToken] = UDPWaitingClientItem{
			lobby:    s.currentLobby,
			position: pos,
		}
		waitingClientsMutex.Unlock()

		s.lastWaitToken = nextToken
		s.SendPackage(Isaacpb.ResponseHeader_ServerUdpToken, 0, &Isaacpb.ResponseServerUdpToken{Token: nextToken})
	case Isaacpb.RequestHeader_LogConsoleChat:
		msg := Isaacpb.RequestLogConsoleChat{}
		if err := proto.Unmarshal(body, &msg); err != nil {
			log.Print(err)
			return errors.New("failed to parse LogConsoleChat package")
		}

		lobbiesMutex.Lock()
		L, ok := lobbies[s.currentLobby]
		lobbiesMutex.Unlock()
		if ok {
			filteredStr := filterStr(msg.Message)

			log.Print("user ", s.name, "(", s.steamId, ") say:", filteredStr, "(", msg.Message, ")")

			L.lobbyMutex.Lock()
			L.SendPackageToAllUsers(Isaacpb.ResponseHeader_LogConsoleChat, 0, &Isaacpb.ResponseLogConsoleChat{
				Steamid: int64(s.steamId),
				Message: filteredStr,
			}, 0)
			L.lobbyMutex.Unlock()
		}
	}
	return nil
}

func (s *SessionData) Close() {
	log.Print("user ", s.name, "(", s.steamId, ") say bye-bye")
	if s.steamId != 0 {
		sessionsMutex.Lock()
		delete(sessions, s.steamId)
		sessionsMutex.Unlock()
	}
	if s.currentLobby != 0 {
		s.LeaveLobby()
	}
}
