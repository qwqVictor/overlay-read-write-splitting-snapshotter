//go:build linux

package snapshotter

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"unsafe"

	"github.com/containerd/containerd/log"
	"github.com/containerd/containerd/mount"
	"github.com/containerd/containerd/snapshots"
	"github.com/containerd/containerd/snapshots/overlay"
	"github.com/containerd/containerd/snapshots/storage"
)

func NewSnapshotter(root string, opts ...overlay.Opt) (snapshots.Snapshotter, error) {
	sn, err := overlay.NewSnapshotter(root, opts...)
	if err != nil {
		return nil, err
	}
	return &overlayReadWriteSplittingSnapshotter{sn, root}, nil
}

// overlayReadWriteSplittingSnapshotter 继承 overlay Snapshotter，在返回 mounts 的地方进行改造
type overlayReadWriteSplittingSnapshotter struct {
	snapshots.Snapshotter
	root string
}

// Mounts implements snapshots.Snapshotter.
func (s *overlayReadWriteSplittingSnapshotter) Mounts(ctx context.Context, key string) ([]mount.Mount, error) {
	mounts, err := s.Snapshotter.Mounts(ctx, key)
	if err != nil {
		return nil, err
	}
	return s.tryManipulate(ctx, key, mounts)
}

func (s *overlayReadWriteSplittingSnapshotter) getMS() (ms *storage.MetaStore) {
	field := reflect.ValueOf(s.Snapshotter).Elem().FieldByName("ms")
	ms = reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Interface().(*storage.MetaStore)
	return
}

func (s *overlayReadWriteSplittingSnapshotter) Remove(ctx context.Context, key string) (err error) {
	var removals []string
	defer func() {
		if err == nil {
			for _, dir := range removals {
				if err := os.RemoveAll(dir); err != nil {
					log.G(ctx).WithError(err).WithField("path", dir).Warn("failed to remove directory")
				}
			}
		}
	}()
	return s.getMS().WithTransaction(ctx, true, func(ctx context.Context) error {
		_, _, err = storage.Remove(ctx, key)
		if err != nil {
			return fmt.Errorf("failed to remove snapshot %s: %w", key, err)
		}

		removals, err = s.getCleanupDirectories(ctx)
		if err != nil {
			return fmt.Errorf("unable to get directories for removal: %w", err)
		}
		return nil
	})
}

func (s *overlayReadWriteSplittingSnapshotter) Cleanup(ctx context.Context) error {
	cleanup, err := s.cleanupDirectories(ctx)
	if err != nil {
		return err
	}

	for _, dir := range cleanup {
		if err := os.RemoveAll(dir); err != nil {
			log.G(ctx).WithError(err).WithField("path", dir).Warn("failed to remove directory")
		}
	}

	return nil
}

func (s *overlayReadWriteSplittingSnapshotter) cleanupDirectories(ctx context.Context) (_ []string, err error) {
	var cleanupDirs []string
	// Get a write transaction to ensure no other write transaction can be entered
	// while the cleanup is scanning.

	if err := s.getMS().WithTransaction(ctx, true, func(ctx context.Context) error {
		cleanupDirs, err = s.getCleanupDirectories(ctx)
		return err
	}); err != nil {
		return nil, err
	}
	return cleanupDirs, nil
}

func (s *overlayReadWriteSplittingSnapshotter) getCleanupDirectories(ctx context.Context) ([]string, error) {
	ids, err := storage.IDMap(ctx)
	if err != nil {
		return nil, err
	}

	snapshotRdDir := filepath.Join(s.root, "snapshots")
	rdfd, err := os.Open(snapshotRdDir)
	if err != nil {
		return nil, err
	}
	defer rdfd.Close()

	rdDirs, err := rdfd.Readdirnames(0)
	if err != nil {
		return nil, err
	}

	snapshotWrDir := filepath.Join(s.root, "writable", "snapshots")
	wrfd, err := os.Open(snapshotWrDir)
	if err != nil {
		return nil, err
	}
	defer wrfd.Close()

	wrDirs, err := wrfd.Readdirnames(0)
	if err != nil {
		return nil, err
	}

	cleanup := []string{}
	for _, d := range rdDirs {
		if _, ok := ids[d]; ok {
			continue
		}
		cleanup = append(cleanup, filepath.Join(snapshotRdDir, d))
	}
	for _, d := range wrDirs {
		if _, ok := ids[d]; ok {
			continue
		}
		cleanup = append(cleanup, filepath.Join(snapshotWrDir, d))
	}

	return cleanup, nil
}

// Prepare implements snapshots.Snapshotter.
func (s *overlayReadWriteSplittingSnapshotter) Prepare(ctx context.Context, key string, parent string, opts ...snapshots.Opt) ([]mount.Mount, error) {
	mounts, err := s.Snapshotter.Prepare(ctx, key, parent, opts...)
	if err != nil {
		return nil, err
	}
	return s.tryManipulate(ctx, key, mounts)
}

// View implements snapshots.Snapshotter.
func (s *overlayReadWriteSplittingSnapshotter) View(ctx context.Context, key string, parent string, opts ...snapshots.Opt) ([]mount.Mount, error) {
	mounts, err := s.Snapshotter.View(ctx, key, parent, opts...)
	if err != nil {
		return nil, err
	}
	return s.tryManipulate(ctx, key, mounts)
}

// tryManipulate 所有返回 mounts 的地方，都需要调用该函数
func (s *overlayReadWriteSplittingSnapshotter) tryManipulate(ctx context.Context, key string, mounts []mount.Mount) ([]mount.Mount, error) {
	// evil hack! 除解压镜像外 containerd 传入的 key 不会在去除命名空间后以 extract- 开头，除非 namespace 包含 '/'，但这不可能
	if len(mounts) != 1 || mounts[0].Type != "overlay" || strings.HasPrefix(key[strings.Index(key, "/"):], "/extract-") {
		return mounts, nil
	}
	for i, o := range mounts[0].Options {
		// fmt.Printf("walked mounts[0].Options[%d]: %v\n", i, o)
		if strings.HasPrefix(o, "upperdir=") && !strings.HasPrefix(o, "upperdir="+filepath.Join(s.root, "writable")) {
			originaldir := strings.TrimPrefix(o, "upperdir=")
			suffix := strings.TrimPrefix(originaldir, s.root)
			upperdir := filepath.Join(s.root, "writable", suffix)
			if err := os.MkdirAll(upperdir, 0o755); err != nil {
				return nil, fmt.Errorf("failed to create upperdir %s: %v", upperdir, err)
			}
			mounts[0].Options[i] = "upperdir=" + upperdir
		}
		if strings.HasPrefix(o, "workdir=") && !strings.HasPrefix(o, "workdir="+filepath.Join(s.root, "writable")) {
			originaldir := strings.TrimPrefix(o, "workdir=")
			suffix := strings.TrimPrefix(originaldir, s.root)
			workdir := path.Join(s.root, "writable", suffix)
			if err := os.MkdirAll(workdir, 0o755); err != nil {
				return nil, fmt.Errorf("failed to create workdir %s: %v", workdir, err)
			}
			mounts[0].Options[i] = "workdir=" + workdir
		}
	}
	return mounts, nil
}
