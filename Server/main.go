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

package main

import (
	"IsaacPaperServer/Isaac"
	"fmt"
	"log"
	"os"
	"regexp"
	"time"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Println("usage: ", os.Args[0], " <admin password>")
		os.Exit(1)
	}

	log.Print("Server protocol version: ", Isaac.PROTOCOL_VER)
	log.Print("Server started, listening tcp port 8555 and udp port 8554...")

	Isaac.LOGIN_PRIVILEDGE = os.Args[1]

	//Isaac.ServeForever("tcp4", "192.168.102.1:8555")
	if re, err := regexp.Compile("^$"); err == nil {
		Isaac.TextFilterRe = re
	}
	go Isaac.ServeUdp("0.0.0.0:8554")

	go func() {
		for {
			time.Sleep(time.Minute * 10)
			n := Isaac.DeleteOldLobbies()
			if n != 0 {
				log.Fatal("Delete ", n, " old lobbies.")
			}
		}
	}()

	Isaac.ServeForever("tcp4", "0.0.0.0:8555")
}
