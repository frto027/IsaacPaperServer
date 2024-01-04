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
	"sync"
)

const (
	USER_MODE_PRIVATE = iota
	USER_MODE_PUBLIC
)

const (
	USER_ACCESS_ALLOW = iota
	USER_ACCESS_DENY
)

type UserAccessInfo struct {
	access      int
	blockReason *string
}

var (
	sessions      = map[SteamID]*SessionData{}
	sessionsMutex = sync.Mutex{}

	lobbies      = map[LobbyID]*LobbyData{}
	lobbiesMutex = sync.Mutex{}

	userAccessMode  = USER_MODE_PUBLIC
	userAccess      = map[SteamID]UserAccessInfo{}
	userAccessMutex = sync.Mutex{}
)
