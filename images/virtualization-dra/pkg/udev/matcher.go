/*
Copyright 2025 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package udev

// Matcher is an interface for filtering uevents
type Matcher interface {
	// Match returns true if the event should be processed
	Match(event *UEvent) bool
}

// MatcherFunc is a function adapter for Matcher
type MatcherFunc func(event *UEvent) bool

// Match implements Matcher interface
func (f MatcherFunc) Match(event *UEvent) bool {
	return f(event)
}

// SubsystemMatcher matches events by subsystem
type SubsystemMatcher struct {
	Subsystem string
}

// Match implements Matcher interface
func (m *SubsystemMatcher) Match(event *UEvent) bool {
	return event.Subsystem() == m.Subsystem
}

// SubsystemDevTypeMatcher matches events by subsystem and device type
type SubsystemDevTypeMatcher struct {
	Subsystem string
	DevType   string
}

// Match implements Matcher interface
func (m *SubsystemDevTypeMatcher) Match(event *UEvent) bool {
	return event.Subsystem() == m.Subsystem && event.DevType() == m.DevType
}

// AndMatcher combines multiple matchers with AND logic
type AndMatcher struct {
	Matchers []Matcher
}

// Match implements Matcher interface
func (m *AndMatcher) Match(event *UEvent) bool {
	for _, matcher := range m.Matchers {
		if !matcher.Match(event) {
			return false
		}
	}
	return true
}

// OrMatcher combines multiple matchers with OR logic
type OrMatcher struct {
	Matchers []Matcher
}

// Match implements Matcher interface
func (m *OrMatcher) Match(event *UEvent) bool {
	for _, matcher := range m.Matchers {
		if matcher.Match(event) {
			return true
		}
	}
	return false
}

// AllMatcher matches all events
type AllMatcher struct{}

// Match implements Matcher interface
func (m *AllMatcher) Match(_ *UEvent) bool {
	return true
}
