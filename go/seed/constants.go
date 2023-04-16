/*
 * Copyright 2019-2021 Yuji Ito <llamerada.jp@gmail.com>
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
package seed

const (
	// version
	ProtocolVersion = "A2"

	// Node ID.
	NidStrNone    = ""
	NidStrThis    = "."
	NidStrSeed    = "seed"
	NidStrNext    = "next"
	NidTypeNone   = 0
	NidTypeNormal = 1
	NidTypeThis   = 2
	NidTypeSeed   = 3
	NidTypeNext   = 4

	// Packet mode.
	ModeNone      = 0x0000
	ModeResponse  = 0x0001
	ModeExplicit  = 0x0002
	ModeOneWay    = 0x0004
	ModeRelaySeed = 0x0008
	ModeNoRetry   = 0x0010

	ChannelNone         = 0
	ChannelMain         = 1
	ChannelSeedAccessor = 2
	ChannelNodeAccessor = 3

	// Commonly packet method.
	MethodError   = 0xffff
	MethodFailure = 0xfffe
	MethodSuccess = 0xfffd

	MethodSeedAuth          = 1
	MethodSeedHint          = 2
	MethodSeedPing          = 3
	MethodSeedRequireRandom = 4

	MethodWebrtcConnectOffer = 1

	// Offer type of WebRTC connect.
	OfferTypeFirst = 0

	// Hint
	HintOnlyOne       = uint32(0x01)
	HintRequireRandom = uint32(0x02)

	// Key of routineLocal
	GroupMutex = 1
	LinkMutex  = 2
)
