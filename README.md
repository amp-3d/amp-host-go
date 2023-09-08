# go-archost
This repo implements `arc.Host` as defined in the [ArcXR SDK](https://github.com/arcspace/go-arc-sdk).  It can be compiled into a binary that either embeds into a Unity or Unreal project, or runs as a standalone "headless" server.  In either case, any `arc.App` can be plugged in, offering many possibilities as to how developers can drive and leverage the ArcXR 3D-based UI/UX.

## Building

Use `make build` to build the `archost` executable and the `libarchost` dynamic libraries for all platforms:

```
$ make help

go-archost
  PARENT_PATH:     /Users/aomeara/git.arcspace
  ARC_SDK_PATH:    /Users/aomeara/git.arcspace/go-arc-sdk
  BUILD_PATH:      /Users/aomeara/git.arcspace/go-archost
  UNITY_PROJ:      /Users/aomeara/git.arcspace/arcspace.unity-app
  UNITY_ARC_LIBS:  /Users/aomeara/git.arcspace/arcspace.unity-app/Assets/Plugins/ArcXR/Plugins
  UNITY_PATH:      /Users/aomeara/Applications/2022.3.8f1
  ANDROID_NDK:     /Users/aomeara/Applications/2022.3.8f1/PlaybackEngines/AndroidPlayer/NDK
  ANDROID_CC:      /Users/aomeara/Applications/2022.3.8f1/PlaybackEngines/AndroidPlayer/NDK/toolchains/llvm/prebuilt/darwin-x86_64/bin

  archost                          builds archost "headless" executable
  build                            builds both archost & libarchost
  generate                         generate .cs and .go files from .proto
  help                             prints this message
  libarchost                       builds libarchost for all platforms for embeddeding in a Unity or Unreal project
  libarchost-android-arm64-v8a     builds libarchost for arm64-v8a
  libarchost-android-armeabi-v7a   builds libarchost for armeabi-v7a 
  libarchost-android-x86_64_       builds libarchost for armeabi-x86_64
  libarchost-ios                   builds libarchost for iOS -- build on x86_64 mac for now
  libarchost-osx                   builds libarchost for OSX -- build on x86_64 mac for now
```

## Running

```
[arc.Host]  task.Context tree:

0001  arc.Host
0002     ┣ AssetServer [::]:5193
0026     ┃    ┗ /Users/aomeara/Movies/Downloads/Alan Watts | Thought and Things.mp4
0003     ┣ grpc.HostService [::]:5192
0015     ┃    ┣ grpc <- HostSession(14)
0016     ┃    ┗ grpc -> HostSession(14)
0014     ┗ HostSession
0017          ┣ app: planet.sys.arcspace.systems
0018          ┃    ┗ planet: aomeara
0019          ┗ app: filesys.bridges.arcspace.systems
0020               ┣ cell: aomeara/
0021               ┃    ┗ [req 1003] arc://filesys?path=/Users/aomeara
0022               ┣ cell: Movies/
0023               ┃    ┗ [req 1004]
0024               ┣ cell: Downloads/
0025               ┃    ┗ [req 1005]
0027               ┗ cell: Alan Watts | Thought and Things.mp4
0028                    ┗ [req 1006]
```


See [arcspace.unity-app](https://github.com/arcspace/arcspace.unity-app) to get started.