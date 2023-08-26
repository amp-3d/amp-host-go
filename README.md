# go-archost
---
### ArcXR and OS ("arcOS") backend runtime for Go


## Upcoming worklist
  - iterate `arc` SI model for string keys that use `SuperTrie` on the client 
  - what thread should PinCell() be running from so that it doesn't block processing of other host msgs. Should be in planet cell's OnRun loop.
  - add 'make backup' -- removes binaries and creates zip of the src
  - add pre buuild step that, for each crate, enables it only for the platform it is for.
  - Adopt capnp -- use struct ID as DataModelD and use string annotgation to tag each filed with one or more pin schema descs: "always", "when-child", "when-pinned", "as-playable"
      - maybe just one string with comma or space separated tag idents? 
    
    // FUTURE: make attrs in the client use SuperTrie (LSM) where the default SI is the empty string.
    //     - makes keying more flexible, more suited to application
    //     - memory efficient 
    // string              SI              = 17;
    

---

See [arcspace.unity-app](https://github.com/arcspace/arcspace.unity-app) to get started.