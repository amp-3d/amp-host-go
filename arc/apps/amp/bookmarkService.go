package amp

import (
	"github.com/arcspace/go-arcspace/arc"
	"github.com/arcspace/go-arcspace/arc/apps/amp/api"
)

func (app *ampApp) pinRadioHome(user arc.User) (arc.AppCell, error) {

	// TODO: the right way to do this is like in the Unity client: register all ValTypes and then dynamically build AttrSpecs
	// Since we're not doing that, for onw just build an AttrSpec from primitive types.

	//usr.MakeSchemaForStruct(app, LoginInfo)

	// Pins the named cell relative to the user's home planet and app URI (guaranteeing app and user scope)
	var login api.LoginInfo
	err := user.ReadCell(app, ".bookmark-server-client-login", &login)
	if err != nil {
		if arc.GetErrCode(err) == arc.ErrCode_CellNotFound {

		}
	}

	return nil, nil
}
