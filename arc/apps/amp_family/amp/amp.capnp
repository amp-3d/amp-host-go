@0xa55d87bf801ad4f7;

using Go = import "/go.capnp";
$Go.package("amp");
$Go.import("github.com/arcspace/go-arc-sdk");

using CSharp = import "/csharp.capnp";
$CSharp.namespace("Arcspace");

# Pinned attr for playableCellSpec
const playableAssetAttrSpec  :Text = "AssetRef:playable";

const playableCellSpec       :Text = "(CellInfo,MediaInfo)(AssetRef:playable)";
const playlistCellSpec       :Text = "(CellInfo,MediaPlaylist)()";


const listItemSeparator :Text = " · ";
