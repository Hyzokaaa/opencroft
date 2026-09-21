package services

import "sort"

// Dependents finds the containers whose databases live inside another one.
//
// It exists because destroying a container that holds somebody else's data is
// the one destruction croft can see coming and nobody else can. The answer is
// computed, never stored: the dependency is recorded only on the side that
// consumes it, so there is no second copy to drift out of agreement with the
// first.
//
// It takes every container's configuration at once because the runtime already
// returns all of it in one listing — asking per container would be the twenty
// calls that reading a container page used to cost.
func Dependents(configs map[string]map[string]string, container string) []string {
	found := []string{}

	for name, config := range configs {
		if name == container {
			continue
		}
		for _, database := range All(config) {
			if database.Location == container {
				found = append(found, name)
				break
			}
		}
	}

	sort.Strings(found)
	return found
}

// Refusal is what the panel says instead of destroying a container others are
// reading from. croft can refuse; it cannot prevent — `lxc delete` is still
// right there, and that is deliberate. What catches the case croft did not get
// asked about is the reconciliation pass, which reports a database reference
// pointing at a container that is gone.
func Refusal(container string, dependents []string) string {
	if len(dependents) == 0 {
		return ""
	}

	subject := "depends"
	if len(dependents) > 1 {
		subject = "depend"
	}

	return join(dependents) + " " + subject + " on a database inside " +
		container + ". Destroying it leaves " + them(len(dependents)) +
		" without data."
}

func join(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	default:
		return joinAll(names)
	}
}

func joinAll(names []string) string {
	out := names[0]
	for i := 1; i < len(names)-1; i++ {
		out += ", " + names[i]
	}
	return out + " and " + names[len(names)-1]
}

func them(n int) string {
	if n == 1 {
		return "it"
	}
	return "them"
}
