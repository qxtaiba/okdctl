package addon

import (
	"fmt"
	"sort"
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

// ResolveByLevel groups addons into parallelism levels using Kahn's algorithm.
// Level 0 contains addons with no dependencies, level 1 depends only on level 0, etc.
// Within each level addons are sorted by priority for deterministic ordering.
func ResolveByLevel(addons []Addon) ([][]Addon, error) {
	if len(addons) == 0 {
		return nil, nil
	}

	byName := make(map[string]Addon, len(addons))
	for _, a := range addons {
		byName[a.Info().Name] = a
	}

	inDegree := make(map[string]int, len(addons))
	dependents := make(map[string][]string)
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

	// Collect initial zero-in-degree nodes
	var queue []Addon
	for _, a := range addons {
		if inDegree[a.Info().Name] == 0 {
			queue = append(queue, a)
		}
	}
	sortByPriority(queue)

	var levels [][]Addon
	processed := 0

	for len(queue) > 0 {
		level := make([]Addon, len(queue))
		copy(level, queue)
		levels = append(levels, level)
		processed += len(level)

		var next []Addon
		for _, current := range queue {
			for _, depName := range dependents[current.Info().Name] {
				inDegree[depName]--
				if inDegree[depName] == 0 {
					next = append(next, byName[depName])
				}
			}
		}
		sortByPriority(next)
		queue = next
	}

	if processed != len(addons) {
		return nil, fmt.Errorf("circular dependency detected among addons")
	}

	return levels, nil
}

func sortByPriority(addons []Addon) {
	sort.Slice(addons, func(i, j int) bool {
		pi := addons[i].Info().Priority
		pj := addons[j].Info().Priority
		if pi != pj {
			return pi < pj
		}
		return addons[i].Info().Name < addons[j].Info().Name
	})
}
