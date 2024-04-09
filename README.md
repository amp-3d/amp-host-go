# amp-host-go
This repo implements `amp.Host` as defined in the [AMP SDK](https://github.com/amp-space/amp-sdk-go).  It can be compiled into a binary that either embeds into a [Unity](https://github.com/amp-space/amp-client-unity) or [Unreal](https://github.com/amp-space/amp-client-unreal) project, or runs as a server launched from command line.  In both cases, any [`amp.App`](https://github.com/amp-space/amp-sdk-go/blob/main/amp/api.app.go) can be plugged in, offering possibilities to how developers can use AMP.  

## Building
Use `make build` to build the `archost` and `arcgate` executables or the `libarchost` dynamic libraries:

```
amp-host-go % make help

  PARENT_PATH:     /Users/aomeara/amp-space
  AMP_SDK_PATH:    ../amp-sdk-go/
  BUILD_PATH:      /Users/aomeara/amp-space/amp-host-go
  UNITY_PROJ:      /Users/aomeara/amp-space/amp-client-unity
  UNITY_AMP_LIBS:  /Users/aomeara/amp-space/amp-client-unity/Assets/Plugins/AMP/Plugins
  UNITY_PATH:      /Users/aomeara/Applications/2022.3.22f1
  ANDROID_NDK:     /Users/aomeara/Applications/2022.3.22f1/PlaybackEngines/AndroidPlayer/NDK
  ANDROID_CC:      /Users/aomeara/Applications/2022.3.22f1/PlaybackEngines/AndroidPlayer/NDK/toolchains/llvm/prebuilt/darwin-x86_64/bin

  arcgate                          builds arcgate executable
  archost                          builds archost executable
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
[amp.Host]  task.Context tree:
0001  amp.Host
0002     ┣ AssetServer [::]:5193
0016     ┃    ┗ /Users/aomeara/Movies/Downloads/Alan Watts Lectures | On Pain.mp4
0003     ┣ tcp.HostService [::]:5192
0005     ┃    ┣ tcp 127.0.0.1:63945 <- amp.HostSession(4)
0006     ┃    ┗ tcp 127.0.0.1:63945 -> amp.HostSession(4)
0004     ┗ amp.HostSession
0007          ┣ app: planet.sys.amp-space.systems
0008          ┃    ┗ planet: aomeara
0009          ┗ app: filesys.bridges.amp-space.systems
0010               ┣ cell: aomeara/
0011               ┃    ┗ [req 1003]  amp://filesys?path=/Users/aomeara
0012               ┣ cell: Movies/
0013               ┃    ┗ [req 1004] 
0014               ┣ cell: Downloads/
0015               ┃    ┗ [req 1005] 
0017               ┗ cell: Alan Watts Lectures | On Pain.mp4
0018                    ┗ [req 1006] 
```


