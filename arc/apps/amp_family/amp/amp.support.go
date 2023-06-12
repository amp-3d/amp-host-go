package amp

import "github.com/arcspace/go-arc-sdk/apis/arc"

func (v *MediaInfo) MarshalToMsg(msg *arc.Msg) error {
	msg.SetupValBuf(arc.ValType(ValTypeAmp_MediaInfo), v.Size())
	_, err := v.MarshalToSizedBuffer(msg.ValBuf)
	return err
}
