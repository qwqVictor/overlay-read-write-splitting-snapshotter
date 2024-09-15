//go:build linux

package snapshotter

const (
	// 改插件默认的存储路径
	DefaultRootDir = "/var/lib/containerd/tech.imvictor.containerd.overlay-read-write-splitting-snapshotter"
	// 该插件提供 grpc 服务的 socks 文件名，路径为 paths.Join(rootDir, SocksFileName)
	// 默认为 /var/lib/containerd/cn.rectcircle.containerd.overlay-custom-add-lower-snapshotter/grpc.socks
	SocksFileName = "grpc.socks"
)
