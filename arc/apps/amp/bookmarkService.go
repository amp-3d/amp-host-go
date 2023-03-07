package amp

import (
	"github.com/arcspace/go-arcspace/arc"
	"github.com/arcspace/go-arcspace/arc/apps/amp/pb"
)

func (app *ampApp) pinRadioHome(user arc.User) (arc.AppPinnedCell, error) {

	// TODO: the right way to do this is like in the Unity client: register all ValTypes and then dynamically build AttrSpecs
	// Since we're not doing that, for onw just build an AttrSpec from primitive types.

	//usr.MakeSchemaForStruct(app, LoginInfo)

	// Pins the named cell relative to the user's home planet and app URI (guaranteeing app and user scope)
	var login pb.LoginInfo
	err := user.ReadCell(app, ".bookmark-server-client-login", &login)
	if err != nil {
		if arc.UnwrapErr(err) == arc.ErrCode_CellNotFound {

		}
	}

	return nil, nil

}
