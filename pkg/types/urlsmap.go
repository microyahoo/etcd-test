package types

import (
	"sort"
	"strings"
)

// URLsMap is a map from a name to its URLs.
type URLsMap map[string]URLs

// NewURLsMap returns a URLsMap instantiated from the given string,
// which consists of discovery-formatted names-to-URLs, like:
// mach0=http://1.1.1.1:2380,mach0=http://2.2.2.2::2380,mach1=http://3.3.3.3:2380,mach2=http://4.4.4.4:2380
func NewURLsMap(s string) (URLsMap, error) {
	m := parse(s)

	cl := URLsMap{}
	for name, urls := range m {
		us, err := NewURLs(urls)
		if err != nil {
			return nil, err
		}
		cl[name] = us
	}
	return cl, nil
}

// URLs returns a list of all URLs.
// The returned list is sorted in ascending lexicographical order.
func (c URLsMap) URLs() []string {
	var urls []string
	for _, us := range c {
		for _, u := range us {
			urls = append(urls, u.String())
		}
	}
	sort.Strings(urls)
	return urls
}

// Len returns the size of URLsMap.
func (c URLsMap) Len() int {
	return len(c)
}

// parse parses the given string and returns a map listing the values specified for each key.
func parse(s string) map[string][]string {
	m := make(map[string][]string)
	for s != "" {
		key := s
		if i := strings.IndexAny(key, ","); i >= 0 {
			key, s = key[:i], key[i+1:]
		} else {
			s = ""
		}
		if key == "" {
			continue
		}
		value := ""
		if i := strings.Index(key, "="); i >= 0 {
			key, value = key[:i], key[i+1:]
		}
		m[key] = append(m[key], value)
	}
	return m
}
