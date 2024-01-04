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
	"bufio"
	list2 "container/list"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	LOGIN_PRIVILEDGE = "NO_DEFAULT_PASSWORD"
)

const (
	UserAuth_NotLogin = iota
	UserAuth_Priviledge
)

type AdminData struct {
	conn   net.Conn
	writer *bufio.Writer
	auth   int
}

func (A *AdminData) SendPackage(text string) error {
	_, err := A.writer.WriteString(text)
	if err != nil {
		return err
	}
	err = A.writer.WriteByte(0)
	if err != nil {
		return err
	}
	err = A.writer.Flush()
	return err
}

func (A *AdminData) HandleSingleCommand(cmd string, args string) {
	switch cmd {
	case "help":
		switch args {
		case "help":
			fallthrough
		default:
			_ = A.SendPackage(`list of commands:
help [cmd]				print help information

broadcast [txt]			send [txt] to all user

info					print current server infos
time					print the current server time
lsuser					print user infos
lslobby					print lobby infos

log [text]				print [text] to the server's logfile
exit					kill this connection
killserver				exit the server process

setroomnames [name1] [name2]...			set room names
setchatbtns	[btn1] [btn2]...			set chat btns
setfilter [filter_regexp]				set text filter

del_old_lobby			delete old empty lobbies

public					set server mode to public
private					set server mode to private
allow [steamid]
deny  [steamid] [reason]
kick  [steamid] [reason]
`)
		}
	case "info":
		sessionsMutex.Lock()
		sessionCount := len(sessions)
		sessionsMutex.Unlock()
		lobbiesMutex.Lock()
		lobbyCount := len(lobbies)
		lobbiesMutex.Unlock()

		str := fmt.Sprint(
			"Protocol version:\t", PROTOCOL_VER, "\n",
			"session count:\t", sessionCount, "\n",
			"lobby count:\t", lobbyCount, "\n",
		)

		DefaultLobbyNamesMutex.Lock()
		if DefaultLobbyNames != nil {
			str += "lobby create names:"
			for _, s := range *DefaultLobbyNames {
				str += fmt.Sprint(s, ",")
			}
			str += "\n"
		}
		DefaultLobbyNamesMutex.Unlock()

		DefaultFastChatMessagesMutex.Lock()
		if DefaultFastChatMessages != nil {
			str += "fast chat messages:"
			for _, s := range *DefaultFastChatMessages {
				str += fmt.Sprint(s, ",")
			}
			str += "\n"
		}
		DefaultFastChatMessagesMutex.Unlock()

		TextFilterReMutex.Lock()
		str += fmt.Sprint("text filter regexp: ", TextFilterRe, "\n")
		TextFilterReMutex.Unlock()

		str += "server access mode: "
		userAccessMutex.Lock()
		switch userAccessMode {
		case USER_MODE_PRIVATE:
			str += "deny all users"
		case USER_MODE_PUBLIC:
			str += "allow all users"
		}
		userAccessMutex.Unlock()
		str += "\n"

		_ = A.SendPackage(str)

	case "killserver":
		log.Print("this server is killed by admin")
		os.Exit(4)
	case "time":
		t := time.Now()
		_ = A.SendPackage(fmt.Sprint("current server time:", t, "(unix:", t.Unix(), ")"))
	case "log":
		log.Print(args)
	case "lsuser", "lsu", "lsus", "lsuse":
		sessionsMutex.Lock()
		i := 0
		for _, s := range sessions {
			_, _ = A.writer.WriteString(fmt.Sprintf("% 4d  %d %s", i, s.steamId, s.name))
			if s.currentLobby != 0 {
				_, _ = A.writer.WriteString(fmt.Sprintf(" at_lobby %d", s.currentLobby))
			}
			i += 1
			_ = A.writer.WriteByte('\n')
		}
		sessionsMutex.Unlock()
		_, _ = A.writer.WriteString("--End Of List--\n")
		_ = A.writer.WriteByte(0)
		_ = A.writer.Flush()
	case "lslobby", "lsl", "lslo", "lslob", "lslobb":
		lobbiesMutex.Lock()
		i := 0
		for _, L := range lobbies {
			PASSWORD := "no password"
			if L.password != nil {
				PASSWORD = "PASSWORD: " + *L.password
			}
			_, _ = A.writer.WriteString(fmt.Sprintf("% 4d  ID:%d NAME: %s %s", i, L.id, L.name, PASSWORD))
			i += 1
			_, _ = A.writer.WriteString("(")
			for j := 0; j < 4; j++ {
				_, _ = A.writer.WriteString(fmt.Sprint(L.users[j], " "))
			}
			_, _ = A.writer.WriteString(")\n")
		}
		lobbiesMutex.Unlock()
		_, _ = A.writer.WriteString("--End Of List--\n")
		_ = A.writer.WriteByte(0)
		_ = A.writer.Flush()
	case "broadcast":
		i := 0
		list := list2.New()
		sessionsMutex.Lock()
		for _, S := range sessions {
			list.PushBack(S)
		}
		sessionsMutex.Unlock()

		caption := "来自管理员的消息"
		pkg := Isaacpb.ResponseServerPublicMessage{
			Type:    Isaacpb.ResponseServerPublicMessage_DisplayStringAtLogConsole,
			Caption: &caption,
			Str:     &args,
		}

		for it := list.Front(); it != nil; it = it.Next() {
			s := it.Value.(*SessionData)
			s.connSendMutex.Lock()
			s.SendPackageNoLock(Isaacpb.ResponseHeader_ServerPublicMessage, 0, &pkg)
			s.connSendMutex.Unlock()
			i++
		}
		_, _ = A.writer.WriteString(fmt.Sprint(i, " users have received the message\n"))
		_ = A.writer.WriteByte(0)
		_ = A.writer.Flush()
	case "setroomnames":
		DefaultLobbyNamesMutex.Lock()
		names := strings.Split(args, " ")
		DefaultLobbyNames = &names
		DefaultLobbyNamesMutex.Unlock()
	case "setchatbtns":
		DefaultFastChatMessagesMutex.Lock()
		btns := strings.Split(args, " ")
		DefaultFastChatMessages = &btns
		DefaultFastChatMessagesMutex.Unlock()
	case "setfilter":
		TextFilterReMutex.Lock()
		if re, err := regexp.Compile(args); err == nil {
			TextFilterRe = re
		}
		TextFilterReMutex.Unlock()
	case "del_old_lobby":
		_ = A.SendPackage(fmt.Sprint("Delete ", DeleteOldLobbies(), "lobbies"))
	case "exit":
		_ = A.conn.Close()
	case "public":
		userAccessMutex.Lock()
		userAccessMode = USER_MODE_PUBLIC
		userAccessMutex.Unlock()
	case "private":
		userAccessMutex.Lock()
		userAccessMode = USER_MODE_PRIVATE
		userAccessMutex.Unlock()
	case "allow":
		id, err := strconv.ParseInt(args, 10, 64)
		if err == nil {
			userAccessMutex.Lock()
			userAccess[SteamID(id)] = UserAccessInfo{access: USER_ACCESS_ALLOW}
			userAccessMutex.Unlock()
			_ = A.SendPackage("success")
		} else {
			_ = A.SendPackage(fmt.Sprint("failed:", err))
		}
	case "deny":
		argss := strings.SplitN(args, " ", 2)
		reason := "您被禁止连接此服务器"
		if len(argss) == 2 {
			reason = argss[1]
		}
		id, err := strconv.ParseInt(argss[0], 10, 64)
		if err == nil {
			userAccessMutex.Lock()
			userAccess[SteamID(id)] = UserAccessInfo{
				access:      USER_ACCESS_DENY,
				blockReason: &reason,
			}
			userAccessMutex.Unlock()
			_ = A.SendPackage("success")
		} else {
			_ = A.SendPackage(fmt.Sprint("failed:", err))
		}
	case "kick":
		reason := "服务器管理员进行了踢出操作"
		argss := strings.SplitN(args, " ", 2)
		if len(argss) == 2 {
			reason = argss[1]
		}
		id, err := strconv.ParseInt(argss[0], 10, 64)
		if err != nil {
			_ = A.SendPackage(fmt.Sprint("failed to convert steam ID:", err))
			return
		}

		sessionsMutex.Lock()
		s, ok := sessions[SteamID(id)]
		sessionsMutex.Unlock()
		if !ok {
			_ = A.SendPackage(fmt.Sprint("session not exist, user is not connected to server"))
		}
		caption := "您被踢出此服务器"
		s.SendPackage(Isaacpb.ResponseHeader_ServerPublicMessage, 0, &Isaacpb.ResponseServerPublicMessage{
			Type:    Isaacpb.ResponseServerPublicMessage_DisplayStringAndExit,
			Str:     &reason,
			Caption: &caption,
		})
	}
}

func (A *AdminData) HandleCommand(text string) error {
	if A.auth == UserAuth_NotLogin {
		time.Sleep(time.Duration(rand.Intn(1000)+1000) * time.Millisecond)

		if text == LOGIN_PRIVILEDGE {
			A.auth = UserAuth_Priviledge
			_ = A.SendPackage(`Welcome to the admin CLI of IsaacPaperServer!
You are authorized as admin. type "help" to display more information. 
`)
			return nil
		}

		return errors.New(fmt.Sprint("user is not auth, from ", A.conn.RemoteAddr()))
	}

	cmd := strings.SplitN(text, " ", 2)
	switch len(cmd) {
	case 1:
		A.HandleSingleCommand(text, "")
	case 2:
		A.HandleSingleCommand(cmd[0], cmd[1])
	}
	return nil
}

func HandleAdminSession(s *SessionData) {
	data := AdminData{
		conn:   s.conn,
		writer: bufio.NewWriter(s.conn),
		auth:   UserAuth_NotLogin,
	}

	reader := bufio.NewReader(data.conn)
	for {
		s, err := reader.ReadString(0)
		if err != nil {
			if err == io.EOF {
				return
			}
			log.Print(err)
			return
		}

		for len(s) > 0 && s[len(s)-1] == 0 {
			s = s[:len(s)-1]
		}

		s = strings.TrimRight(s, "\r\n")

		if err := data.HandleCommand(s); err != nil {
			return
		}
	}
}
