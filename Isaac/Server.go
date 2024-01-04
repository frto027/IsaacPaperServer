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
	list2 "container/list"
	"encoding/binary"
	"log"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"google.golang.org/protobuf/proto"
)

var DefaultLobbyNames *[]string = nil
var DefaultLobbyNamesMutex = sync.Mutex{}

var DefaultFastChatMessages *[]string = nil
var DefaultFastChatMessagesMutex = sync.Mutex{}

var TextFilterRe *regexp.Regexp
var TextFilterReMutex = sync.Mutex{}

func filterStr(s string) string {
	r := s
	TextFilterReMutex.Lock()
	if TextFilterRe != nil {
		r = TextFilterRe.ReplaceAllStringFunc(s, func(s string) string {
			return strings.Repeat("\uF004", utf8.RuneCountInString(s))
		})
	}
	TextFilterReMutex.Unlock()
	return r
}

func ServeTcp(conn net.Conn) {
	buff := make([]byte, 4096)
	session := SessionData{}
	session.Create(conn)
	defer func(conn net.Conn) {
		_ = conn.Close()
	}(conn)
	defer session.Close()

	for {
		size_bts := make([]byte, 4)
		readed := uint32(0)
		for readed < 4 {
			r, err := conn.Read(size_bts[readed:4])
			if err != nil {
				log.Print(err)
				return
			}
			if r < 0 {
				log.Print("Invalid read")
				return
			}
			readed += uint32(r)
		}

		size := binary.BigEndian.Uint32(size_bts)
		if size >= 4096 {
			log.Print("size", size, "is too big!")
			_ = conn.Close()
			return
		}

		readed = 0
		for readed < size {
			r, err := conn.Read(buff[readed:size])
			if err != nil {
				log.Print(err)
				return
			}
			if r < 0 {
				log.Print("Invalid read!")
				return
			}
			readed += uint32(r)
		}

		header := Isaacpb.RequestHeader{}
		err := proto.Unmarshal(buff[0:size], &header)
		if err != nil {
			return
		}

		size = uint32(header.Length)
		readed = 0
		for readed < size {
			r, err := conn.Read(buff[readed:size])
			if err != nil {
				log.Print(err)
				return
			}
			if r < 0 {
				log.Print("Invalid read!")
				return
			}
			readed += uint32(r)
		}
		if err := session.HandlePackage(&header, buff[0:size]); err != nil {
			log.Print("session closed: ", err)
			return
		}
	}
}

func ServeForever(network string, addr string) {
	listener, err := net.Listen(network, addr)
	if err != nil {
		log.Fatal(err)
	}
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Print(err)
		}
		go ServeTcp(conn)
	}
}

func DeleteOldLobbies() int {
	deleteIfBefore := time.Now().Add(-time.Minute)
	count := 0
	toDel := list2.New()
	lobbiesMutex.Lock()
	for ID, L := range lobbies {
		if L.UserCount() == 0 && L.createTime.Before(deleteIfBefore) {
			toDel.PushBack(ID)
			count++
		}
	}
	for e := toDel.Front(); e != nil; e = e.Next() {
		delete(lobbies, e.Value.(LobbyID))
	}
	lobbiesMutex.Unlock()
	return count
}
