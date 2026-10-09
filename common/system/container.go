package system

import (
	"os"
	"strings"
	"sync"
)

// IsRunningInContainer 是否运行在容器中（Docker / Kubernetes / containerd 等），结果在进程内缓存。
var IsRunningInContainer = sync.OnceValue(func() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return true
	}
	data, err := os.ReadFile("/proc/1/cgroup")
	if err != nil {
		return false
	}
	content := string(data)
	for _, marker := range []string{"docker", "kubepods", "containerd", "lxc"} {
		if strings.Contains(content, marker) {
			return true
		}
	}
	return false
})
