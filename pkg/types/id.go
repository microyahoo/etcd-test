package types

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// ID represents a generic identifier which is canonically
// stored as a uint64 but is typically represented as a
// base-16 string for input/output
type ID uint64

func (i ID) String() string {
	return strconv.FormatUint(uint64(i), 16)
}

// IDFromString attempts to create an ID from a base-16 string.
func IDFromString(s string) (ID, error) {
	i, err := strconv.ParseUint(s, 16, 64)
	return ID(i), err
}

// IDSlice implements the sort interface
type IDSlice []ID

func (p IDSlice) Len() int           { return len(p) }
func (p IDSlice) Less(i, j int) bool { return uint64(p[i]) < uint64(p[j]) }
func (p IDSlice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

type URLs []url.URL

func NewURLs(strs []string) (URLs, error) {
	all := make([]url.URL, len(strs))
	if len(all) == 0 {
		return nil, errors.New("no valid URLs given")
	}
	for i, in := range strs {
		in = strings.TrimSpace(in)
		u, err := url.Parse(in)
		if err != nil {
			return nil, err
		}
		if u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "unix" && u.Scheme != "unixs" {
			return nil, fmt.Errorf("URL scheme must be http, https, unix, or unixs: %s", in)
		}
		if _, _, err := net.SplitHostPort(u.Host); err != nil {
			return nil, fmt.Errorf(`URL address does not have the form "host:port": %s`, in)
		}
		if u.Path != "" {
			return nil, fmt.Errorf("URL must not contain a path: %s", in)
		}
		all[i] = *u
	}
	us := URLs(all)
	us.Sort()

	return us, nil
}

func (us URLs) String() string {
	return strings.Join(us.StringSlice(), ",")
}

func (us *URLs) Sort() {
	sort.Sort(us)
}
func (us URLs) Len() int           { return len(us) }
func (us URLs) Less(i, j int) bool { return us[i].String() < us[j].String() }
func (us URLs) Swap(i, j int)      { us[i], us[j] = us[j], us[i] }

func (us URLs) StringSlice() []string {
	out := make([]string, len(us))
	for i := range us {
		out[i] = us[i].String()
	}

	return out
}
