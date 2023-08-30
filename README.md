# go-archost
---
### ArcXR / OS backend runtime for Go


```

	archost task.Context tree:
	
0001  arc.Host
0002     ┣ AssetServer [::]:5193
0003     ┣ grpc.HostService [::]:5192
0013     ┃    ┣ grpc <- HostSession(12)
0014     ┃    ┗ grpc -> HostSession(12)
0012     ┗ HostSession
0015          ┣ app: planet.sys.arcspace.systems
0016          ┃    ┗ planet: aomeara
0018          ┃         ┗ cell: 22e0000022f
0017          ┗ app: spotify.amp.arcspace.systems
0021               ┣ cell: Spotify Home
0022               ┃    ┗ req 1003: arc://spotify/home
0023               ┣ cell: Followed Playlists
0024               ┃    ┗ req 1005
0028               ┗ cell: Followed Artists
0029                    ┗ req 1008
```

## Worklist
  - iterate `arc` SI model for string keys that use `SuperTrie` on the client 
  - add 'make backup' -- removes binaries and creates zip of the src
  - add pre build step that, for each crate, enables it only for the platform it is for.
  - have 'make generate' find all the .proto files and generate the .pb..go file for them
  - finalize TxID scheme, allowing removal TargetCell (CellID) from CellTx.-- 16 SeriesIndex values?
    

---

See [arcspace.unity-app](https://github.com/arcspace/arcspace.unity-app) to get started.