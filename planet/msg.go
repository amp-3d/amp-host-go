package planet

import (
	"reflect"
	"sync"

	"github.com/genesis3systems/go-planet/symbol"
)

// Sets a reasonable size beyond which buffers should be shared rather than copied.
const MsgValBufCopyLimit = 16 * 1024

func NewMsgBatch() MsgBatch {
	batch := gMsgBatchPool.Get().(*msgBatch)
	return batch
}

type msgBatch struct {
	msgs []*Msg
}

func (batch *msgBatch) AddNew(count int) []*Msg{
	N := len(batch.msgs)
	for i := 0; i < count; i++ {
		batch.msgs = append(batch.msgs, NewMsg())
	}
	return batch.msgs[N:]
}

func (batch *msgBatch) AddMsgs(msgs []*Msg) {
	batch.msgs = append(batch.msgs, msgs...)
}

func (batch *msgBatch) AddMsg(msg *Msg) {
	batch.msgs = append(batch.msgs, msg)
}

func (batch *msgBatch) Msgs() []*Msg {
	return batch.msgs
}

func (batch *msgBatch) Reclaim() {
	for i, msg := range batch.msgs {
		msg.Reclaim()
		batch.msgs[i] = nil
	}
	batch.msgs = batch.msgs[:0]
}

func NewMsg() *Msg {
	msg := gMsgPool.Get().(*Msg)
	if msg.ValBufIsShared {
		panic("Msg discarded as shared")
	}
	return msg
}

func CopyMsg(src *Msg) *Msg {
	msg := NewMsg()

	if src != nil {
		// If the src buffer is big share it instead of copy it
		if len(src.ValBuf) > MsgValBufCopyLimit {
			*msg = *src
			msg.ValBufIsShared = true
			src.ValBufIsShared = true
		} else {
			valBuf := append(msg.ValBuf[:0], src.ValBuf...)
			*msg = *src
			msg.ValBufIsShared = false
			msg.ValBuf = valBuf
		}
	}
	return msg
}

func (msg *Msg) Reclaim() {
	if msg != nil {
		if msg.ValBufIsShared {
			*msg = Msg{}
		} else {
			valBuf := msg.ValBuf[:0]
			*msg = Msg{
				ValBuf: valBuf,
			}
		}
		gMsgPool.Put(msg)
	}
}

func (msg *Msg) SetValBuf(valType ValType, sz int) {

	msg.ValInt = int64(sz)
	msg.ValType = uint64(valType)
	if sz > cap(msg.ValBuf) {
		msg.ValBuf = make([]byte, sz, (sz+0x3FF)&^0x3FF)
	} else {
		msg.ValBuf = msg.ValBuf[:sz]
	}

}

func (msg *Msg) SetVal(value interface{}) {
	var err error

	switch v := value.(type) {
	
	case string:
		msg.SetValBuf(ValType_string, len(v))
		copy(msg.ValBuf, v)

	case *Defs:
		msg.SetValBuf(ValType_Defs, v.Size())
		_, err = v.MarshalToSizedBuffer(msg.ValBuf)

		// case *CellInfo:
		// 	msg.SetValBuf(uint64(ValType_CellInfo), v.Size())
		//     _, err = v.MarshalToSizedBuffer(msg.ValBuf)

	case *Err:
		msg.SetValBuf(ValType_Err, v.Size())
		_, err = v.MarshalToSizedBuffer(msg.ValBuf)

	case error:
		plErr, _ := v.(*Err)
		if plErr == nil {
			err := ErrCode_UnnamedErr.Wrap(v)
			plErr = err.(*Err)
		}
		msg.SetValBuf(ValType_Err, plErr.Size())
		_, err = plErr.MarshalToSizedBuffer(msg.ValBuf)

	case nil:
		msg.SetValBuf(ValType_nil, 0)
	}

	if err != nil {
		panic(err)
	}
}

func (msg *Msg) LoadVal(dst interface{}) error {
	if msg == nil {
		loadNil(dst)
		return ErrCode_BadValue.Error("got nil Msg")
	}

	ok := false

	switch msg.ValType {

	// case uint64(Type_CellInfo):
	//     if v, ok := dst.(*CellInfo); ok {
	//         info := CellInfo{}
	//         if info.Unmarshal(msg.ValBuf) == nil {
	//             *v = info
	//             ok = true
	//         }
	//     }
	case uint64(ValType_PinReq):
		if v, match := dst.(*PinReq); match {
			tmp := PinReq{}
			if tmp.Unmarshal(msg.ValBuf) == nil {
				*v = tmp
				ok = true
			}
		}

	case uint64(ValType_Defs):
		if v, match := dst.(*Defs); match {
			tmp := Defs{}
			if tmp.Unmarshal(msg.ValBuf) == nil {
				*v = tmp
				ok = true
			}
		}
		
	case uint64(ValType_LoginReq):
		if v, match := dst.(*LoginReq); match {
			tmp := LoginReq{}
			if tmp.Unmarshal(msg.ValBuf) == nil {
				*v = tmp
				ok = true
			}
		}

	}

	if !ok {
		return ErrCode_BadValue.Errorf("expected %v from Msg", reflect.TypeOf(dst))
	}

	return nil
}

func loadNil(dst interface{}) {
	switch v := dst.(type) {
	// case *Content:
	//     v.ContentType = v.ContentType[:0]
	//     v.DataLen = 0
	//     v.Data = v.Data[:0]
	case *string:
		*v = ""
	case *symbol.ID:
		*v = 0
	case *TIDBuf:
		*v = TIDBuf{}
	case *TID:
		*v = nil
	// case *PinReq:
	//     *v = PinReq{}
	// case *CellInfo:
	// 	*v = CellInfo{}
	case *int:
		*v = 0
	case *int64:
		*v = 0
	case *uint64:
		*v = 0
	case *float64:
		*v = 0
	case *[]byte:
		*v = nil
	default:
		panic("unexpected dst type")
	}
}

var gMsgPool = sync.Pool{
	New: func() interface{} {
		return new(Msg)
	},
}

var gMsgBatchPool = sync.Pool{
	New: func() interface{} {
		return new(msgBatch)
	},
}

func NewMsgWithValue(value interface{}) *Msg {
	msg := NewMsg()
	msg.SetVal(value)
	return msg
}
