package ip_region

import (
	"fmt"
	"strconv"
	"sync"

	"github.com/lionsoul2014/ip2region/binding/golang/service"
)

var (
	globalSvc  *service.Ip2Region
	globalOnce sync.Once
	globalLoad sync.Mutex
	globalErr  error
	// cacheKey 记录当前全局单例使用的路径（v4|v6）与缓存策略，路径变化时重建
	cacheKey string
)

// buildService 根据 v4/v6 库路径与缓存策略构建双栈查询服务。
// v4Path/v6Path 任一为空表示对应协议族未启用（依赖官方 service 包按 IP 版本自动路由）。
func buildService(v4Path, v6Path string, useCache bool) (*service.Ip2Region, error) {
	var policy int
	if useCache {
		// 与旧版 LoadContentFromFile 语义一致：整库加载进内存，IO 最快
		policy = service.BufferCache
	} else {
		// 与旧版 NewWithFileOnly 语义一致：不缓存，文件 IO 查询
		policy = service.NoCache
	}

	// service 包要求 searchers > 0；BufferCache 模式该值被忽略（走 in-memory searcher）
	const searchers = 20

	var v4Cfg, v6Cfg *service.Config
	var err error

	if v4Path != "" {
		v4Cfg, err = service.NewV4Config(policy, v4Path, searchers)
		if err != nil {
			return nil, fmt.Errorf("failed to create v4 config from %s: %w", strconv.Quote(v4Path), err)
		}
	}

	if v6Path != "" {
		v6Cfg, err = service.NewV6Config(policy, v6Path, searchers)
		if err != nil {
			return nil, fmt.Errorf("failed to create v6 config from %s: %w", strconv.Quote(v6Path), err)
		}
	}

	svc, err := service.NewIp2Region(v4Cfg, v6Cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create dual-stack ip2region service: %w", err)
	}

	return svc, nil
}

// getSearcher 获取查询服务。
// useCache=true 时全局复用（内存常驻）；否则每次构建并交由调用方 Close。
func getSearcher(v4Path, v6Path string, useCache bool) (*service.Ip2Region, error) {
	if !useCache {
		return buildService(v4Path, v6Path, false)
	}

	key := v4Path + "|" + v6Path
	globalLoad.Lock()
	defer globalLoad.Unlock()

	if globalSvc != nil && cacheKey == key && globalErr == nil {
		return globalSvc, nil
	}

	// 路径或错误状态变化时重建
	globalOnce = sync.Once{}
	globalSvc = nil
	globalErr = nil
	globalOnce.Do(func() {
		globalSvc, globalErr = buildService(v4Path, v6Path, true)
	})
	cacheKey = key

	return globalSvc, globalErr
}

func search(ip, v4Path, v6Path string, useCache bool) (string, error) {
	svc, err := getSearcher(v4Path, v6Path, useCache)
	if err != nil {
		return "", err
	}

	if !useCache { // 未使用缓存查询后释放以节省内存
		defer svc.Close()
	}

	return svc.Search(ip)
}

