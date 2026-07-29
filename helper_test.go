package main

import (
	"testing"
)

func Test_isValidIPv4(t *testing.T) {
	data := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "valid IP", input: "192.168.1.1", want: true},                       // valid IP
		{name: "network address, still valid IP", input: "10.0.0.0", want: true},   // network address, still valid IP
		{name: "broadcast address", input: "255.255.255.255", want: true},          // broadcast address
		{name: "network address, still valid IP", input: "0.0.0.0", want: true},    // network address, still valid IP
		{name: "CIDR notation, not bare IP", input: "192.168.1.1/24", want: false}, // CIDR notation, not bare IP
		{name: "octet > 255", input: "256.0.0.1", want: false},                     // octet > 255
		{name: "missing octet", input: "192.168.1", want: false},                   // missing octet
		{name: "IPv6", input: "::1", want: false},                                  // IPv6
		{name: "empty string", input: "", want: false},                             // empty string
		{name: "CIDR notation, not bare IP", input: "192.168.1.1/24", want: false}, // CIDR notation, not bare IP
	}

	for _, d := range data {
		t.Run(d.name, func(t *testing.T) {
			got := isValidIPv4(d.input)
			if got != d.want {
				t.Errorf("isValidIPv4(%s) = %v, want %v", d.input, got, d.want)
			}
		})
	}
}

func Test_sameIPSet(t *testing.T) {
	data := []struct {
		name     string
		resolved []string
		existing []string
		want     bool
	}{
		{
			name:     "同じ集合・同じ順序",
			resolved: []string{"203.0.113.1/32", "203.0.113.2/32"},
			existing: []string{"203.0.113.1/32", "203.0.113.2/32"},
			want:     true,
		},
		{
			name:     "同じ集合・順序違い",
			resolved: []string{"203.0.113.1/32", "203.0.113.2/32"},
			existing: []string{"203.0.113.2/32", "203.0.113.1/32"},
			want:     true,
		},
		{
			name:     "重複を無視して同じ",
			resolved: []string{"203.0.113.1/32", "203.0.113.1/32"},
			existing: []string{"203.0.113.1/32"},
			want:     true,
		},
		{
			name:     "両方空",
			resolved: []string{},
			existing: []string{},
			want:     true,
		},
		{
			name:     "IPが違う",
			resolved: []string{"203.0.113.1/32"},
			existing: []string{"203.0.113.9/32"},
			want:     false,
		},
		{
			name:     "個数が違う",
			resolved: []string{"203.0.113.1/32"},
			existing: []string{"203.0.113.1/32", "203.0.113.2/32"},
			want:     false,
		},
		{
			name:     "一方だけ空",
			resolved: []string{"203.0.113.1/32"},
			existing: []string{},
			want:     false,
		},
	}

	for _, d := range data {
		t.Run(d.name, func(t *testing.T) {
			got := sameIPSet(d.resolved, d.existing)
			if got != d.want {
				t.Errorf("sameIPSet(%v, %v) = %v, want %v", d.resolved, d.existing, got, d.want)
			}
		})
	}
}
