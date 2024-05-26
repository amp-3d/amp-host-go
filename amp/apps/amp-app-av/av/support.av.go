package av

import (
	"github.com/amp-3d/amp-sdk-go/amp"
)

/*
 */

func (v *LoginInfo) MarshalToStore(dst []byte) ([]byte, error) {
	return amp.MarshalPbToStore(v, dst)
}

func (v *LoginInfo) ElemTypeName() string {
	return "LoginInfo"
}

func (v *LoginInfo) New() amp.ElemVal {
	return &LoginInfo{}
}

func (v *PlayableMedia) MarshalToStore(dst []byte) ([]byte, error) {
	return amp.MarshalPbToStore(v, dst)
}

func (v *PlayableMedia) ElemTypeName() string {
	return "PlayableMedia"
}

func (v *PlayableMedia) New() amp.ElemVal {
	return &PlayableMedia{}
}

func (v *MediaPlaylist) MarshalToStore(dst []byte) ([]byte, error) {
	return amp.MarshalPbToStore(v, dst)
}

func (v *MediaPlaylist) ElemTypeName() string {
	return "MediaPlaylist"
}

func (v *MediaPlaylist) New() amp.ElemVal {
	return &MediaPlaylist{}
}
