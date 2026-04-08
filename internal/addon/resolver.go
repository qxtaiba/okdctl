package addon

import (
	"fmt"
	"slices"
	"strings"
)

// Resolve returns addons in dependency-safe installation order using Kahn's algorithm.
// Addons with no dependency relationship are ordered by priority (lower first).
// Returns an error if there are circular dependencies or missing dependencies.
func Resolve(addons []Addon) ([]Addon, error) {
	if len(addons) == 0 {
		return nil, nil
	}

	byName := make(map[string]Addon, len(addons))
	for _, a := range addons {
		byName[a.Info().Name] = a
	}

	inDegree := make(map[string]int, len(addons))
	dependents := make(map[string][]string) // dependency → addons that depend on it
	for _, a := range addons {
		info := a.Info()
		inDegree[info.Name] = 0
	}
	for _, a := range addons {
		info := a.Info()
		for _, dep := range info.Dependencies {
			if _, ok := byName[dep]; !ok {
				return nil, fmt.Errorf("addon %q depends on %q which is not enabled", info.Name, dep)
			}
			inDegree[info.Name]++
			dependents[dep] = append(dependents[dep], info.Name)
		}
	}

	var queue []Addon
	for _, a := range addons {
		if inDegree[a.Info().Name] == 0 {
			queue = append(queue, a)
		}
	}
	sortByPriority(queue)

	var ordered []Addon
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		ordered = append(ordered, current)

		var unblocked []Addon
		for _, depName := range dependents[current.Info().Name] {
			inDegree[depName]--
			if inDegree[depName] == 0 {
				unblocked = append(unblocked, byName[depName])
			}
		}

		sortByPriority(unblocked)
		queue = append(queue, unblocked...)
	}

	if len(ordered) != len(addons) {
		return nil, fmt.Errorf("circular dependency detected among addons")
	}

	return ordered, nil
}

func sortByPriority(addons []Addon) {
	slices.SortFunc(addons, func(a, b Addon) int {
		if a.Info().Priority != b.Info().Priority {
			return a.Info().Priority - b.Info().Priority
		}
		return strings.Compare(a.Info().Name, b.Info().Name)
	})
}
