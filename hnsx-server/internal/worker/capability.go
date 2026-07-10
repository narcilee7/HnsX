package worker

import (
	"sort"

	pb "github.com/hnsx-io/hnsx/server/proto/gen/go/hnsx/v1"
)

// CapabilitiesFromInfo derives scheduler capability tags from a worker's
// WorkerInfo declaration. The returned list is sorted and deduplicated.
func CapabilitiesFromInfo(info *pb.WorkerInfo) []string {
	if info == nil {
		return nil
	}
	caps := map[string]struct{}{}

	if info.Capacity != nil {
		for _, p := range info.Capacity.Providers {
			if p != "" {
				caps["provider:"+p] = struct{}{}
			}
		}
		for _, m := range info.Capacity.Models {
			if m != "" {
				caps["model:"+m] = struct{}{}
			}
		}
		for _, s := range info.Capacity.SandboxRuntimes {
			if s != "" {
				caps["sandbox:"+s] = struct{}{}
			}
		}
	}

	for k, v := range info.Labels {
		if k != "" {
			caps["label:"+k+":"+v] = struct{}{}
		}
	}

	out := make([]string, 0, len(caps))
	for c := range caps {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}
