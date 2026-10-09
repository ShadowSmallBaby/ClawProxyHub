package account

import "golang.org/x/sys/unix"

// Android 禁止应用创建硬链接，用不可覆盖重命名原子发布已同步的密钥。
func publishKey(source, target string) error {
	return unix.Renameat2(unix.AT_FDCWD, source, unix.AT_FDCWD, target, unix.RENAME_NOREPLACE)
}
