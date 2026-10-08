package service

import (
	"regexp"
	"sync"
	"sync/atomic"
)

const maxCompiledRegexCacheSize = 512

var (
	compiledRegexCache     sync.Map // map[string]*regexp.Regexp
	compiledRegexCacheSize atomic.Int64
)

func matchAnyRegex(patterns []string, s string) bool {
	if len(patterns) == 0 || s == "" {
		return false
	}
	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}
		re, ok := compiledRegexCache.Load(pattern)
		if !ok {
			compiled, err := regexp.Compile(pattern)
			if err != nil {
				// Treat invalid patterns as non-matching to avoid breaking runtime traffic.
				continue
			}
			if compiledRegexCacheSize.Load() >= maxCompiledRegexCacheSize {
				// Bound memory usage under adversarial configs: fall back to
				// direct matching without growing the cache.
				if compiled.MatchString(s) {
					return true
				}
				continue
			}
			actual, loaded := compiledRegexCache.LoadOrStore(pattern, compiled)
			if !loaded {
				compiledRegexCacheSize.Add(1)
			}
			re = actual
		}
		if re.(*regexp.Regexp).MatchString(s) {
			return true
		}
	}
	return false
}
