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
	"errors"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"
)

type LobbyID uint64

func (s *LobbyID) SetID(id uint32) {
	*s = LobbyID(id) // Currently we don't support any lobby property
}

type LobbyData struct {
	users      [4]SteamID
	owner      SteamID
	id         LobbyID
	data       map[string]string // Okay, no need to move it away from server...
	memberData [4]map[string]string
	lobbyMutex sync.Mutex

	udpAddresses [4]netip.AddrPort

	name      string
	password  *string
	enableP2P bool

	createTime time.Time
}

var nextLobbyID = uint32(1)

func (L *LobbyData) Create() {
	L.id.SetID(atomic.AddUint32(&nextLobbyID, 1))
	L.data = make(map[string]string)
	L.memberData = [4]map[string]string{{}, {}, {}, {}}
	L.lobbyMutex = sync.Mutex{}
	L.udpAddresses = [4]netip.AddrPort{{}, {}, {}, {}}
	L.createTime = time.Now()
}
func (L *LobbyData) HasUser(user SteamID) bool {
	for _, u := range L.users {
		if u == user {
			return true
		}
	}
	return false
}
func (L *LobbyData) UserCount() int {
	i := 0
	for _, u := range L.users {
		if u != 0 {
			i += 1
		}
	}
	return i
}
func (L *LobbyData) UserPosition(user SteamID) int {
	for i, u := range L.users {
		if u == user {
			return i
		}
	}
	return -1
}
func (L *LobbyData) AddUser(user SteamID) (int, error) {
	pos := L.UserPosition(user)
	if pos != -1 {
		return pos, nil // already in, returns the existing position
	}
	for i, u := range L.users {
		if u == 0 {
			L.users[i] = user
			return i, nil
		}
	}
	return -1, errors.New("lobby is full")
}
func (L *LobbyData) RemoveUser(user SteamID) {
	idx := L.UserPosition(user)
	if idx != -1 {
		L.users[idx] = 0
		L.udpAddresses[idx] = netip.AddrPort{}
	}
	if L.owner == user {
		L.owner = 0
		// try to find a new owner
		for _, u := range L.users {
			if u != 0 {
				L.owner = u
			}
		}
	}
}

func (L *LobbyData) IsEmpty() bool {
	for _, user := range L.users {
		if user != 0 {
			return false
		}
	}
	return true
}

func (L *LobbyData) ToProtobufLobbyInfo() *Isaacpb.LobbyInfo {
	L.lobbyMutex.Lock()
	info := Isaacpb.LobbyInfo{}
	info.LobbyId = uint64(L.id)
	info.OwnerId = uint64(L.owner)
	info.UserIds = []uint64{uint64(L.users[0]), uint64(L.users[1]), uint64(L.users[2]), uint64(L.users[3])}
	info.Datas = make([]*Isaacpb.LobbyDataUpdateItem, len(L.data))
	info.Name = L.name
	info.HasPassword = L.password != nil
	idx := 0
	for k, v := range L.data {
		info.Datas[idx] = &Isaacpb.LobbyDataUpdateItem{K: k, V: v}
		idx++
	}
	L.lobbyMutex.Unlock()
	return &info
}
func (L *LobbyData) ToProtobufLobbyInfoWithUserData() *Isaacpb.LobbyInfo {
	info := L.ToProtobufLobbyInfo()
	L.lobbyMutex.Lock()
	info.UsersDatas = make([]*Isaacpb.SingleUserData, 4)
	for i := 0; i < 4; i++ {
		singleData := Isaacpb.SingleUserData{}
		info.UsersDatas[i] = &singleData
		if L.users[i] == 0 {
			continue
		}

		if L.enableP2P && L.udpAddresses[i].IsValid() {
			info.UsersDatas[i].UdpIpAddr = L.udpAddresses[i].Addr().AsSlice()
			info.UsersDatas[i].UdpPort = int32(L.udpAddresses[i].Port())
		} else {
			info.UsersDatas[i].UdpIpAddr = make([]byte, 0)
			info.UsersDatas[i].UdpPort = 0
		}

		singleData.Data = make([]*Isaacpb.SingleUserDataItem, len(L.memberData[i]))
		idx := 0
		for k, v := range L.memberData[i] {
			singleData.Data[idx] = &Isaacpb.SingleUserDataItem{K: k, V: v}
			idx++
		}
	}
	L.lobbyMutex.Unlock()
	return info
}
