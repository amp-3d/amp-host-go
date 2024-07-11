package av

import (
	"github.com/amp-3d/amp-sdk-go/amp"
	"github.com/amp-3d/amp-sdk-go/stdlib/tag"
)

func (v *LoginInfo) MarshalToStore(dst []byte) ([]byte, error) {
	return amp.MarshalPbToStore(v, dst)
}

func (v *LoginInfo) TagSpec() tag.Spec {
	return amp.AttrSpec.With("LoginInfo")
}

func (v *LoginInfo) New() tag.Value {
	return &LoginInfo{}
}

func (v *MediaTrackInfo) MarshalToStore(dst []byte) ([]byte, error) {
	return amp.MarshalPbToStore(v, dst)
}

func (v *MediaTrackInfo) TagSpec() tag.Spec {
	return amp.AttrSpec.With("MediaTrackInfo")
}

func (v *MediaTrackInfo) New() tag.Value {
	return &MediaTrackInfo{}
}
