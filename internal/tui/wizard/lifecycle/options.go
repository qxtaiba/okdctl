package lifecycle

import "github.com/qxtaiba/okdctl/internal/node"

// ResizeOptionsFrom maps the wizard-collected state onto the backend's
// resize options. Host-budget fields are merged by the CLI, which owns the
// probe results.
func ResizeOptionsFrom(st *State) node.ResizeOptions {
	return node.ResizeOptions{
		MemoryMB:    st.MemoryMB,
		CPU:         st.CPU,
		SkipDrain:   st.SkipDrain,
		Acknowledge: st.Ack,
	}
}

// RemoveOptionsFrom maps the wizard-collected state onto the backend's
// remove options.
func RemoveOptionsFrom(st *State) node.RemoveOptions {
	return node.RemoveOptions{
		ForceStorage: st.ForceStorage,
		SkipDrain:    st.SkipDrain,
		DrainTimeout: st.DrainTimeout,
		Acknowledge:  st.Ack,
	}
}

// AddOptionsFrom maps the wizard-collected state onto the backend's add
// options. Host-budget fields are merged by the CLI.
func AddOptionsFrom(st *State) node.AddOptions {
	return node.AddOptions{
		Count:       st.Count,
		Acknowledge: st.Ack,
	}
}
