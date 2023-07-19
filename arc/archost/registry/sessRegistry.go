package registry

import (
	"runtime"
	"sync/atomic"
	"time"

	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-arc-sdk/stdlib/symbol"
	"github.com/arcspace/go-archost/arc/archost/registry/parse"
)

type elemDef struct {
	prototype arc.ElemVal
}

type sessRegistry struct {
	table symbol.Table

	timeMu atomic.Int32 // mutex for issuing time IDs
	timeID int64        // previously issued time ID

	nativeToClientID map[uint32]uint32 // maps a native symbol ID to a client symbol ID
	clientToNativeID map[uint32]uint32 // maps a client symbol ID to a native symbol ID

	elemDefs map[uint32]elemDef     // by ElemTypeID by native ID
	attrDefs map[uint32]arc.AttrDef // by AttrID by native ID
	cellDefs map[uint32]arc.CellDef // by CellTypeID by native ID
}

func NewSessionRegistry(table symbol.Table) arc.SessionRegistry {
	reg := &sessRegistry{
		table:            table,
		nativeToClientID: make(map[uint32]uint32),
		clientToNativeID: make(map[uint32]uint32),

		elemDefs: make(map[uint32]elemDef),
		attrDefs: make(map[uint32]arc.AttrDef),
		cellDefs: make(map[uint32]arc.CellDef),
	}
	arc.RegisterConstSymbols(reg)
	return reg
}

func (reg *sessRegistry) ClientSymbols() symbol.Table {
	return reg.table
}

func (reg *sessRegistry) NativeToClientID(nativeID uint32) (uint32, bool) {
	clientID, ok := reg.nativeToClientID[nativeID]
	return clientID, ok
}

func (reg *sessRegistry) IssueTimeID() arc.TimeID {
	now := int64(arc.ConvertToUTC(time.Now()))

	if !reg.timeMu.CompareAndSwap(0, 1) { // spin lock
		runtime.Gosched()
	}
	issued := reg.timeID
	if issued < now {
		issued = now
	} else {
		issued += 1
	}
	reg.timeID = issued
	reg.timeMu.Store(0) // spin unlock

	return arc.TimeID(issued)
}

func (reg *sessRegistry) NewAttrElem(attrID uint32, native bool) (arc.ElemVal, error) {
	if !native {
		clientID := attrID
		attrID = reg.clientToNativeID[clientID]
		if attrID == 0 {
			return nil, arc.ErrCode_DefNotFound.Errorf("NewAttrElem: unrecognized client symbol %v", clientID)
		}
	}
	// Often, an attrID will be a unnamed scalar attr (which means we can get the elemDef directly.
	// This is also essential during bootstrapping when the client sends a RegisterDefs is not registered yet.
	elemDef, exists := reg.elemDefs[attrID]
	if !exists {
		attrDef, exists := reg.attrDefs[attrID]
		if !exists {
			return nil, arc.ErrCode_DefNotFound.Errorf("NewAttrElem: attr DefID %v not found", attrID)
		}
		elemDef, exists = reg.elemDefs[attrDef.Native.ElemType]
		if !exists {
			return nil, arc.ErrCode_DefNotFound.Errorf("NewAttrElem: elemTypeID %v not found", attrDef.Client.ElemType)
		}
	}
	return elemDef.prototype.New(), nil
}

func (reg *sessRegistry) RegisterElemType(proto arc.ElemVal) error {

	// If the client symbol for this type is not present, defer registration of this prototype
	elemType := proto.TypeName()
	nativeElemID := reg.resolveNative(elemType)
	def, exists := reg.elemDefs[nativeElemID]
	if !exists {
		def.prototype = proto
		reg.elemDefs[nativeElemID] = def
	}
	return nil
}

func (reg *sessRegistry) RegisterDefs(defs *arc.RegisterDefs) error {

	//
	//
	// Symbols
	for _, sym := range defs.Symbols {
		nativeID := uint32(reg.table.GetSymbolID(sym.Name, true))
		if clientID := reg.nativeToClientID[nativeID]; clientID != 0 && clientID != sym.ID {
			return arc.ErrCode_BadSchema.Errorf("client symbol %q already registered as %v ", sym.Name, clientID)
		}
		reg.nativeToClientID[nativeID] = sym.ID
		reg.clientToNativeID[sym.ID] = nativeID
	}

	//
	//
	// AttrSpecs
	for _, attrSpec := range defs.Attrs {
		def := arc.AttrDef{
			Client: *attrSpec,
			Native: arc.AttrSpec{
				DefID:           reg.clientToNativeID[attrSpec.DefID],
				AttrName:        reg.clientToNativeID[attrSpec.AttrName],
				ElemType:        reg.clientToNativeID[attrSpec.ElemType],
				SeriesSpec:      reg.clientToNativeID[attrSpec.SeriesSpec],
				SeriesIndexType: attrSpec.SeriesIndexType,
			},
		}

		switch {
		case def.Client.AttrName == 0 && def.Native.AttrName != 0:
			return arc.ErrCode_BadSchema.Errorf("RegisterDefs: AttrSpec %v failed to resolve AttrName", def.Client.DefID)
		case def.Client.ElemType == 0 && def.Native.ElemType != 0:
			return arc.ErrCode_BadSchema.Errorf("RegisterDefs: AttrSpec %v failed to resolve ElemType", def.Client.DefID)
		case def.Client.SeriesSpec == 0 && def.Native.SeriesSpec != 0:
			return arc.ErrCode_BadSchema.Errorf("RegisterDefs: AttrSpec %v failed to resolve SeriesSpec", def.Client.DefID)
		case def.Native.DefID == 0:
			return arc.ErrCode_BadSchema.Errorf("RegisterDefs: AttrSpec %v failed to resolve DefID", def.Client.DefID)
		}

		// In the case an attr was already registered natively, we want to still overwrite since the client IDs will now be available.
		reg.attrDefs[def.Native.DefID] = def
	}

	//
	//
	// CellSpecs
	for _, cellSpec := range defs.Cells {
		def := arc.CellDef{
			ClientDefID: cellSpec.DefID,
			NativeDefID: reg.clientToNativeID[cellSpec.DefID],
		}
		reg.cellDefs[def.NativeDefID] = def
	}

	return nil
}

func (reg *sessRegistry) resolveNative(name string) uint32 {
	return uint32(reg.table.GetSymbolID([]byte(name), true))
}

func (reg *sessRegistry) ResolveAttrSpec(attrSpec string, native bool) (def arc.AttrSpec, err error) {
	expr, err := parse.AttrSpecParser.ParseString("", attrSpec)
	if err != nil {
		return arc.AttrSpec{}, err
	}

	spec := arc.AttrSpec{
		DefID:           reg.resolveNative(attrSpec),
		AttrName:        reg.resolveNative(expr.AttrName),
		ElemType:        reg.resolveNative(expr.ElemType),
		SeriesSpec:      reg.resolveNative(expr.SeriesSpec),
		SeriesIndexType: arc.GetSeriesIndexType(expr.SeriesSpec),
	}

	// If resolving a native-only attr spec, also register it since RegisterDefs() is for client defs only.
	if native {
		prev, exists := reg.attrDefs[spec.DefID]
		if exists {
			if prev.Native != spec {
				err = arc.ErrCode_BadSchema.Errorf("ResolveAttrSpec: native AttrSpec %v already registered with different fields ", spec.DefID)
			}
		} else {
			// If the client also registers the this attr spec later, the client portion will be updated.
			reg.attrDefs[spec.DefID] = arc.AttrDef{
				Native: spec,
			}
		}
	} else {
		clientSpec := arc.AttrSpec{
			DefID:           reg.nativeToClientID[spec.DefID],
			AttrName:        reg.nativeToClientID[spec.AttrName],
			ElemType:        reg.nativeToClientID[spec.ElemType],
			SeriesSpec:      reg.nativeToClientID[spec.SeriesSpec],
			SeriesIndexType: spec.SeriesIndexType,
		}

		switch {
		case clientSpec.AttrName == 0 && spec.AttrName != 0:
			err = arc.ErrCode_BadSchema.Errorf("ResolveAttrSpec: failed to resolve %q for AttrSpec %q", expr.AttrName, attrSpec)
		case clientSpec.ElemType == 0 && spec.ElemType != 0:
			err = arc.ErrCode_BadSchema.Errorf("ResolveAttrSpec: failed to resolve %q for AttrSpec %q", expr.ElemType, attrSpec)
		case clientSpec.SeriesSpec == 0 && spec.SeriesSpec != 0:
			err = arc.ErrCode_BadSchema.Errorf("ResolveAttrSpec: failed to resolve %q for AttrSpec %q", expr.SeriesSpec, attrSpec)
		case clientSpec.DefID == 0:
			err = arc.ErrCode_BadSchema.Errorf("ResolveAttrSpec: failed to resolve %q", attrSpec)
		}

		spec = clientSpec
	}

	return spec, err
}

func (reg *sessRegistry) ResolveCellSpec(cellSpec string) (arc.CellDef, error) {
	// TODO: build parser for CellSpec -- for now just assume cellSpec is canonic and good to go

	nativeSpecID := reg.resolveNative(cellSpec)
	//def, exists := reg.cellDefs[nativeSpecID]

	def := arc.CellDef{
		NativeDefID: nativeSpecID,
		ClientDefID: reg.nativeToClientID[nativeSpecID],
	}

	var err error
	if def.ClientDefID == 0 {
		err = arc.ErrCode_BadSchema.Errorf("ResolveCellSpec: failed to resolve %q", cellSpec)
	}

	return def, err
}

/*
func (reg *sessRegistry) resolveSymbol(sym *Symbol, autoIssue bool) error {
	reg.tokMu.RLock()
	sym.ID = reg.tokCache[string(sym.Name)]
	reg.tokMu.RUnlock()
	if sym.ID != 0 {
		return nil
	}
	sym.ID = uint32(reg.table.GetSymbolID(sym.Name, autoIssue))
	if sym.ID != 0 {
		reg.tokMu.Lock()
		reg.tokCache[string(sym.Name)] = sym.ID
		reg.tokMu.Unlock()
	}
	return nil
}


func (reg *sessRegistry) FormAttr(attrName string, val ElemVal) (AttrElem, error) {
	spec := AttrSpec{
		AttrName: attrName,
		ElemType: val.TypeName(),
	}
	if err := reg.ResolveAttr(&spec, false); err != nil {
		return AttrElem{}, err
	}

	return AttrElem{
		Value:  val,
		AttrID: spec.AttrID,
	}, nil
}


func (reg *sessRegistry) NewAttrForID(attrID uint32) (AttrElem, error) {
	reg.typesMu.RLock()
	typ, found := reg.types[attrID]
	reg.typesMu.RUnlock()
	if found {
		return AttrElem{}, arc.ErrCode_BadSchema.Errorf("unknown attr ID %v", attrID)
	}

	return AttrElem{
		Value:  typ.elemVal.New(),
		AttrID: attrID,
	}, nil
}

func (reg *sessRegistry) ResolveAttr(spec *AttrSpec, autoIssue bool) error {
	var typedName string
	hasName := len(spec.AttrName) > 0
	if hasName {
		typedName = spec.AttrName + spec.ElemType
	}

	reg.tokMu.RLock()
	elemID := reg.tokCache[spec.ElemType]
	if hasName {
		spec.AttrID = reg.tokCache[typedName]
	} else {
		spec.AttrID = elemID
	}
	reg.tokMu.RUnlock()

	if elemID != 0 && spec.AttrID != 0 {
		spec.ElemTypeID = elemID
		return nil
	}

	// The above is the hot path and so if it's not found, retroactively check for bad syntax.
	if autoIssue {
		if strings.ContainsAny(spec.AttrName, "./ ") {
			return arc.ErrCode_BadSchema.Errorf("illegal attr name: %q", spec.AttrName)
		}
		if strings.ContainsAny(spec.ElemType, "/ ") || len(spec.ElemType) <= 2 || spec.ElemType[0] != '.' {
			return arc.ErrCode_BadSchema.Errorf("illegal type name: %q", spec.ElemType)
		}
	} else {
		if spec.ElemType == "" {
			return arc.ErrCode_BadSchema.Error("missing AttrSpec.ElemType")
		}
	}

	gotElem := false
	gotName := false
	if elemID == 0 {
		elemID = uint32(reg.table.GetSymbolID([]byte(spec.ElemType), autoIssue))
		gotElem = elemID != 0
	}
	if spec.AttrID == 0 {
		if hasName {
			spec.AttrID = uint32(reg.table.GetSymbolID([]byte(typedName), autoIssue))
			gotName = spec.AttrID != 0
		} else {
			spec.AttrID = elemID
		}
	}

	if gotName || gotElem {
		reg.tokMu.Lock()
		if gotElem {
			spec.ElemTypeID = elemID
			reg.tokCache[spec.ElemType] = elemID
		}
		if gotName {
			reg.tokCache[typedName] = spec.AttrID
		}
		reg.tokMu.Unlock()
	}

	if !gotName || !gotElem {
		return arc.ErrCode_BadSchema.Errorf("failed to resolve AttrSpec %v", spec)
	}

	return nil
}

func (reg *sessRegistry) RegisterAttrType(attrName string, prototype ElemVal) error {
	spec := AttrSpec{
		AttrName: attrName,
		ElemType: prototype.TypeName(),
	}
	err := reg.ResolveAttr(&spec, true)
	if err != nil {
		return err
	}

	reg.typesMu.Lock()
	reg.types[spec.AttrID] = attrType{
		//attrName: attrName,
		elemVal: prototype,
	}
	reg.typesMu.Unlock()

	return nil
}

// func (attr *AttrSpec) String() string {
//     var buf [128]byte
//     str := fmt.Appendf(buf[:0], "AttrSpec{AttrID:%v, TypedName:%q, ValTypeID:%v, SymbolID:%v}", attr.AttrID, attr.TypedName, attr.ValTypeID, attr.SymbolID)
//     return string(str)
// }


func (reg *sessRegistry) registerAttr(attr *AttrSpec) error {


		if !cleanURI(&attr.AttrName) {
			return arc.ErrCode_BadSchema.Errorf("missing AttrSpec.TypedName in attr %q", attr.String())
		}

		if attr.AttrID == 0 {
			return arc.ErrCode_BadSchema.Errorf("missing AttrSpec.AttrID in attr %q", attr.TypedName)
		}

		if attr.AttrSymID == 0 {
			attr.AttrSymID = reg.table.GetSymbolID([]byte(attr.TypedName), true).Ord()
		}

		if attr.SeriesType != SeriesType_Fixed && attr.BoundSI != 0 {
			return arc.ErrCode_BadSchema.Errorf("AttrSpec.BoundSI is set but is ignored in attr %q", attr.TypedName)
		}

		{
			extPos := strings.IndexByte(attr.TypedName, '.')
			if extPos < 0 {
				return arc.ErrCode_BadSchema.Errorf("missing type suffix in %q", attr.TypedName)
			}
			typeName := attr.TypedName[extPos:]
			typeID := reg.table.GetSymbolID([]byte(typeName), true).Ord()
			if attr.ValTypeID == 0 {
				attr.ValTypeID = typeID
			} else {
				if attr.ValTypeID != typeID {
					return arc.ErrCode_BadSchema.Errorf("AttrSpec.ValTypeID (%v) for type %q does not match the registered type (%v)", attr.ValTypeID, typeID, typeID)
				}
			}
		}

		def := reg.attrsBySymbol[attr.AttrSymID]
		if def != nil {
			// TODO: greenlight multiple definitions of the same attr that are indentical
			return arc.ErrCode_BadSchema.Errorf("duplicate AttrID %v", attr.AttrID)
		}
		reg.attrsBySymbol[attr.AttrSymID] = &attrDef{
			spec: *attr,
		}
		reg.attrsByName[attr.TypedName] = attrEntry{
			attrID: attr.AttrID,
			symID:  attr.AttrSymID,
		}

		// if !cleanURI(&attr.ValTypeURI) {
		// 	return arc.ErrCode_BadSchema.Errorf("missing Attrs[%d].ValTypeURI in schema %s for attr %s", i, schema.SchemaDesc(), attr.AttrURI)
		// }

		// if attr.ValTypeID == 0 {
		// 	attr.ValTypeID = uint64(reg.table.GetSymbolID([]byte(attr.ValTypeURI), true))
		// }


	// // Reorder attrs by ascending AttrID for canonic (and efficient) db access
	// // NOTE: This is for a db symbol lookup table for the schema, not for the client-level declaration
	// sort.Slice(schema.Attrs, func(i, j int) bool {
	// 	return schema.Attrs[i].AttrID < schema.Attrs[j].AttrID
	// })
}



func extractTypeName(attr *AttrSpec) (string, error) {
	extPos := strings.IndexByte(attr.TypedName, '.')
	if extPos < 0 {
		return "", arc.ErrCode_BadSchema.Errorf("missing type suffix in %q", attr.TypedName)
	}
	typeName := attr.TypedName[:extPos]
}



func (reg *sessRegistry) tryResolveDefs(defs []CellDef) error {

    progress := -1
    var unresolved int

    // Remove defs as they able to be registered
    for progress != 0 {
        progress = 0
        unresolved = -1

        for i, def := range defs {
            if def.Spec == nil || def.Spec.Resolved {
                continue
            }

            spec := reg.tryResolve(def.Spec)
            if spec == nil {
                if unresolved < 0 {
                    unresolved = i
                }
                continue
            }

            // TODO -- the proper way to do do this is to:
            //   1) resolve all symbol names into IDs
            //   2) output a canonical text-based spec for def.Spec
            //   3) hash (2) into MD5 etc
            //   4) if (3) already exists, use the already-existing NodeSpec
            //      else, issue a new NodeSpec ID and associate with (3)
            //
            // Until the above is done, we just assume there are no issues and register as we go along.
            def.TypeName = spec.NodeTypeName
            def.Spec = spec
            defs[i] = def
            reg.defs[spec.NodeTypeID] = def
            if reg.nameLookup != nil {
                reg.nameLookup[def.TypeName] = def.Spec.NodeTypeID
            }

            progress++
        }
    }

    if unresolved >= 0 {
        return arc.ErrCode_NodeTypeNotRegistered.ErrWithMsgf("failed to resolve NodeSpec %q", defs[unresolved].TypeName)
    }

    return nil
}




func (schema *AttrSchema) SchemaDesc() string {
	return path.Join(schema.CellDataModel, schema.SchemaName)
}

func (schema *AttrSchema) LookupAttr(typedName string) *AttrSpec {
	for _, attr := range schema.Attrs {
		if attr.TypedName == typedName {
			return attr
		}
	}
	return nil
}

func MakeSchemaForType(valTyp reflect.Type) (*AttrSchema, error) {
	numFields := valTyp.NumField()

	schema := &AttrSchema{
		CellDataModel: valTyp.Name(),
		SchemaName:    "on-demand-reflect",
		Attrs:         make([]*AttrSpec, 0, numFields),
	}

	for i := 0; i < numFields; i++ {

		// Importantly, AttrID is always set to the field index + 1, so we know what field to inspect when given an AttrID.
		field := valTyp.Field(i)
		if !field.IsExported() {
			continue
		}

		attr := &AttrSpec{
			TypedName: field.Name,
			AttrID:  int32(i + 1),
		}

		attrType := field.Type
		attrKind := attrType.Kind()
		switch attrKind {
		case reflect.Int32, reflect.Uint32, reflect.Int64, reflect.Uint64:
			attr.ValTypeID = int32(ValType_int)
		case reflect.String:
			attr.ValTypeID = int32(ValType_string)
		case reflect.Slice:
			elementType := attrType.Elem().Kind()
			switch elementType {
			case reflect.Uint8, reflect.Int8:
				attr.ValTypeID = int32(ValType_bytes)
			}
		}

		if attr.ValTypeID == 0 {
			return nil, arc.ErrCode_ExportErr.Errorf("unsupported type '%s.%s (%v)", schema.CellDataModel, attr.TypedName, attrKind)
		}

		schema.Attrs = append(schema.Attrs, attr)
	}
	return schema, nil
}

// ReadCell loads a cell with the given URI having the inferred schema (built from its fields using reflection).
// The URI is scoped into the user's home planet and AppID.
func ReadCell(ctx AppContext, subKey string, schema *AttrSchema, dstStruct any) error {

	dst := reflect.Indirect(reflect.ValueOf(dstStruct))
	switch dst.Kind() {
	case reflect.Pointer:
		dst = dst.Elem()
	case reflect.Struct:
	default:
		return arc.ErrCode_ExportErr.Errorf("expected struct, got %v", dst.Kind())
	}

	var keyBuf [128]byte
	cellKey := append(append(keyBuf[:0], []byte(ctx.StateScope())...), []byte(subKey)...)

	msgs := make([]*Msg, 0, len(schema.Attrs))
	err := ctx.User().HomePlanet().ReadCell(cellKey, schema, func(msg *Msg) {
		switch msg.Op {
		case MsgOp_PushAttr:
			msgs = append(msgs, msg)
		}
	})
	if err != nil {
		return err
	}

	numFields := dst.NumField()
	valType := dst.Type()

	for fi := 0; fi < numFields; fi++ {
		field := valType.Field(fi)
		for _, ai := range schema.Attrs {
			if ai.TypedName == field.Name {
				for _, msg := range msgs {
					if msg.AttrID == ai.AttrID {
						msg.LoadVal(dst.Field(fi).Addr().Interface())
						goto nextField
					}
				}
			}
		}
	nextField:
	}
	return err
}

// WriteCell is the write analog of ReadCell.
func WriteCell(ctx AppContext, subKey string, schema *AttrSchema, srcStruct any) error {

	src := reflect.Indirect(reflect.ValueOf(srcStruct))
	switch src.Kind() {
	case reflect.Pointer:
		src = src.Elem()
	case reflect.Struct:
	default:
		return arc.ErrCode_ExportErr.Errorf("expected struct, got %v", src.Kind())
	}

	{
		tx := NewMsgBatch()
		msg := tx.AddMsg()
		msg.Op = MsgOp_UpsertCell
		msg.ValType = ValType_SchemaID.Ord()
		msg.ValInt = int64(schema.SchemaID)
		msg.ValBuf = append(append(msg.ValBuf[:0], []byte(ctx.StateScope())...), []byte(subKey)...)

		numFields := src.NumField()
		valType := src.Type()

		for _, attr := range schema.Attrs {
			msg := tx.AddMsg()
			msg.Op = MsgOp_PushAttr
			msg.AttrID = attr.AttrID
			for i := 0; i < numFields; i++ {
				if valType.Field(i).Name == attr.TypedName {
					msg.setVal(src.Field(i).Interface())
					break
				}
			}
			if msg.ValType == ValType_nil.Ord() {
				panic("missing field")
			}
		}

		msg = tx.AddMsg()
		msg.Op = MsgOp_Commit

		if err := ctx.User().HomePlanet().PushTx(tx); err != nil {
			return err
		}
	}

	return nil
}


func (req *CellReq) GetKwArg(argKey string) (string, bool) {
	for _, arg := range req.Args {
		if arg.Key == argKey {
			if arg.Val != "" {
				return arg.Val, true
			}
			return string(arg.ValBuf), true
		}
	}
	return "", false
}

func (req *CellReq) GetChildSchema(modelURI string) *AttrSchema {
	for _, schema := range req.ChildSchemas {
		if schema.CellDataModel == modelURI {
			return schema
		}
	}
	return nil
}

func (req *CellReq) PushBeginPin(target CellID) {
	m := NewMsg()
	m.CellID = target.U64()
	m.Op = MsgOp_PinCell
	req.PushUpdate(m)
}

func (req *CellReq) PushInsertCell(target CellID, schema *AttrSchema) {
	if schema != nil {
		m := NewMsg()
		m.CellID = target.U64()
		m.Op = MsgOp_InsertChildCell
		m.ValType = int32(ValType_SchemaID)
		m.ValInt = int64(schema.SchemaID)
		req.PushUpdate(m)
	}
}

// Pushes the given attr to the client
func (req *CellReq) PushAttr(target CellID, schema *AttrSchema, attrURI string, val Value) {
	attr := schema.LookupAttr(attrURI)
	if attr == nil {
		return
	}

	m := NewMsg()
	m.CellID = target.U64()
	m.Op = MsgOp_PushAttr
	m.AttrID = attr.AttrID
	if attr.SeriesType == SeriesType_Fixed {
		m.SI = attr.BoundSI
	}
	val.MarshalToMsg(m)
	if attr.ValTypeID != 0 { // what is this for!?
		m.ValType = int32(attr.ValTypeID)
	}
	req.PushUpdate(m)
}

func (req *CellReq) PushCheckpoint(err error) {
	m := NewMsg()
	m.Op = MsgOp_Commit
	m.CellID = req.PinCell.U64()
	if err != nil {
		m.setVal(err)
	}
	req.PushUpdate(m)
}

*/

// type Serializable interface {
// 	MarshalToBuf(dst *[]byte) error
// 	Unmarshal(src []byte) error
// }

// type Value2[T any] interface {
// 	Serializable
// }

// type ElemValType[V Serializable] interface {
// 	New() V // Returns a new default instance of this ElemVal type
// 	TypeName() string
// 	//MarshalToBuf(src V, dst *[]byte) error
// 	//UnmarshalBuf(src []byte, dst V) error
// }

// type ElemValTypeBase[V Serializable] struct {

// }

// func (typ ElemValTypeBase[V]) New() V {
//     return V{}
// }

// type CellInfoType struct {}

// func (typ CellInfoType) TypeName() string {
//     return ".CellInfo"
// }

// func (typ CellInfoType) New() ElemVal {
//     return &CellInfo{}
// }

// func (typ ElemValType[V]) New() V {
// 	return typ.New()
// }

// type CellInfoType struct {
//     ElemValType[*CellInfo]
// }

// func (typ CellInfoType) New() *CellInfo {
//     return &CellInfo{}
// }

// func (typ CellInfoType) TypeName() string {
//     return ".CellInfo"
// }
