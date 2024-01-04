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
	"log"
	"net"
	"net/netip"
	"sync"
)

type UDPRemoteClient struct {
	addr    netip.AddrPort
	lobbyId LobbyID
}

var clients = map[netip.AddrPort]*UDPRemoteClient{}
var clientsMutex = sync.Mutex{}

type UDPWaitingClientItem struct {
	lobby    LobbyID
	position int
}

var waitingClients = map[string]UDPWaitingClientItem{}
var waitingClientsMutex = sync.Mutex{}

var conn *net.UDPConn

func (C *UDPRemoteClient) onPackageReceive(bts []byte) {
	if len(bts) == 0 {
		return
	}
	switch bts[0] & 0xF0 {
	case byte(Isaacpb.UdpMessageType_ForwardOrYours), byte(Isaacpb.UdpMessageType_ForwardOrYoursAndChannel),
		byte(Isaacpb.UdpMessageType_EnsurePkg):
		target := bts[0] & 0x3
		lobbiesMutex.Lock()
		lobby, ok := lobbies[C.lobbyId]
		lobbiesMutex.Unlock()
		if ok {
			lobby.lobbyMutex.Lock()
			targetAddr := lobby.udpAddresses[target]
			lobby.lobbyMutex.Unlock()
			if targetAddr.IsValid() {
				_, _ = conn.WriteToUDP(bts, net.UDPAddrFromAddrPort(targetAddr))
			}
		}
	case byte(Isaacpb.UdpMessageType_PingPong):
		_, _ = conn.WriteToUDP([]byte{byte(Isaacpb.UdpMessageType_PingPong)}, net.UDPAddrFromAddrPort(C.addr))
	}
}

func ServeUdp(addr string) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		log.Fatal(err)
	}
	conn, err = net.ListenUDP("udp", udpAddr)
	for {
		buff := make([]byte, 1024*1024)
		n, addr, err := conn.ReadFromUDP(buff)
		if err != nil {
			log.Fatal(err)
		}

		clientsMutex.Lock()
		client, ok := clients[addr.AddrPort()]
		clientsMutex.Unlock()
		if ok {
			client.onPackageReceive(buff[:n])
		}

		//receive a package from unknown client
		waitingClientsMutex.Lock()
		if n < 100 {
			str := string(buff[:n])
			lobby, ok := waitingClients[str]
			if ok {
				delete(waitingClients, str)

				clients[addr.AddrPort()] = &UDPRemoteClient{
					addr:    addr.AddrPort(),
					lobbyId: lobby.lobby,
				}
				waitingClientsMutex.Unlock()

				lobbiesMutex.Lock()
				L, ok := lobbies[lobby.lobby]
				lobbiesMutex.Unlock()
				if ok {
					L.lobbyMutex.Lock()
					L.udpAddresses[lobby.position] = addr.AddrPort()

					L.SendPackageToAllUsers(Isaacpb.ResponseHeader_UpdateUserUdpIpAddr, 0, &Isaacpb.ResponseUserAddr{
						Lobbypos:  int32(lobby.position),
						UdpIpAddr: addr.IP,
						UdpPort:   int32(addr.Port),
					}, 0)
					L.lobbyMutex.Unlock()

					log.Print("user from address ", addr, " has connect udp socket.(lobby ",
						lobby.lobby, ", ", lobby.position, ")")

					PingPongPkg := make([]byte, 1)
					PingPongPkg[0] = byte(Isaacpb.UdpMessageType_PingPong)
					_, _ = conn.WriteToUDP(PingPongPkg, addr)
				}
			} else {
				waitingClientsMutex.Unlock()
			}
		} else {
			waitingClientsMutex.Unlock()
		}

	}
}
