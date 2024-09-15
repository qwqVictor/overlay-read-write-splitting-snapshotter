# overlay-read-write-splitting-snapshot

一个支持读写分离的 OverlayFS snapshotter，作为 containerd 外部 gRPC 插件存在。

An OverlayFS snapshotter that splits readonly and writable layers, as an external gRPC plugin for containerd.

## 使用

**适用场景**：容器镜像较大，但又期望容器可写层性能更好。若将快照都存放在高性能磁盘上则十分浪费储存资源，读写分离可以很好地降低成本。

使用 Go 1.21 以上编译，将程序作为 daemon 启动后，自动创建 `/var/lib/containerd/tech.imvictor.containerd.overlay-read-write-splitting-snapshotter`。

将可写层文件系统挂载于 `/var/lib/containerd/tech.imvictor.containerd.overlay-read-write-splitting-snapshotter/writable`，即可享受读写分离！

在 `/etc/containerd/config.toml` 需要配置：
```toml
[proxy_plugins]
  [proxy_plugins.overlay-read-write-splitting-snapshotter]
    type = "snapshot"
    address = "/var/lib/containerd/tech.imvictor.containerd.overlay-read-write-splitting-snapshotter/grpc.socks"
```
若使用 Kubernetes，还需要按以下 TOML 路径配置：
```toml
[plugins]
  [plugins."io.containerd.grpc.v1.cri"]
    [plugins."io.containerd.grpc.v1.cri".containerd]
      snapshotter = "overlay-read-write-splitting-snapshotter"
```

若从其他 snapshotter 迁移，请确保将 containerd 目录完全清空，否则可能出现不可预料的问题。

### 效果

这是 Kubernetes 宿主机上的展示效果：
```
root@NAS:/var/lib/containerd # mount | grep /var/lib/containerd/tech.imvictor.containerd.overlay-read-write-splitting-snapshotter | egrep -v '^overlay'
/dev/zd512 on /var/lib/containerd/tech.imvictor.containerd.overlay-read-write-splitting-snapshotter type ext4 (rw,relatime,stripe=2)
/dev/zd528 on /var/lib/containerd/tech.imvictor.containerd.overlay-read-write-splitting-snapshotter/writable type ext4 (rw,relatime,stripe=2)
root@NAS:/var/lib/containerd # df -h /var/lib/containerd/tech.imvictor.containerd.overlay-read-write-splitting-snapshotter/writable
Filesystem      Size  Used Avail Use% Mounted on
/dev/zd528       69G  226M   65G   1% /var/lib/containerd/tech.imvictor.containerd.overlay-read-write-splitting-snapshotter/writable
root@NAS:/var/lib/containerd # df -h /var/lib/containerd/tech.imvictor.containerd.overlay-read-write-splitting-snapshotter
Filesystem      Size  Used Avail Use% Mounted on
/dev/zd512       98G   26G   68G  28% /var/lib/containerd/tech.imvictor.containerd.overlay-read-write-splitting-snapshotter
root@NAS:/var/lib/containerd # kubectl exec -it busybox -- sh
/ # df -h /
Filesystem                Size      Used Available Use% Mounted on
overlay                  68.4G    225.2M     64.6G   0% /
/ # mount
overlay on / type overlay (rw,relatime,lowerdir=/var/lib/containerd/tech.imvictor.containerd.overlay-read-write-splitting-snapshotter/snapshots/131/fs,upperdir=/var/lib/containerd/tech.imvictor.containerd.overlay-read-write-splitting-snapshotter/writable/snapshots/141/fs,workdir=/var/lib/containerd/tech.imvictor.containerd.overlay-read-write-splitting-snapshotter/writable/snapshots/141/work)
```

## 原理和参考

该 snapshotter 大致继承于 overlayfs snapshotter，通过劫持 mount 列表和特化回收逻辑，并对 containerd 操作进行判断，从而在容器启动时将可写层和只读层分离。

特别感谢 [rectcircle/overlay-custom-add-lower-snapshotter](https://github.com/rectcircle/overlay-custom-add-lower-snapshotter)，为本项目工程框架提供复用能力，且作者的[博文](https://www.rectcircle.cn/posts/containerd-5-custom-snapshotter/)提供了很多思路。