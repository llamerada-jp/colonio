/*
 * Copyright 2017- Yuji Ito <llamerada.jp@gmail.com>
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
package signal

import (
	proto "github.com/llamerada-jp/colonio/api/colonio/v1alpha"
)

type OfferType int
type AnswerStatus int

const (
	OfferTypeExplicit = OfferType(proto.SignalOfferType_SIGNAL_OFFER_TYPE_EXPLICIT)
	OfferTypeNext     = OfferType(proto.SignalOfferType_SIGNAL_OFFER_TYPE_NEXT)
)

const (
	AnswerStatusReject = iota
	AnswerStatusAccept
)

type Offer struct {
	OfferID   uint32
	OfferType OfferType
	Sdp       string
}

type Answer struct {
	OfferID uint32
	Status  AnswerStatus
	Sdp     string
}

type ICE struct {
	OfferID uint32
	Ices    []string
}
