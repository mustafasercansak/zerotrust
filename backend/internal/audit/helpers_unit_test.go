package audit

import "testing"

func TestQueryInt(t *testing.T) {
	tests := []struct {
		name string
		in   string
		def  int
		want int
	}{
		{name: "valid", in: "50", def: 25, want: 50},
		{name: "negative", in: "-3", def: 25, want: 25},
		{name: "invalid", in: "x", def: 25, want: 25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := queryInt(tt.in, tt.def); got != tt.want {
				t.Fatalf("queryInt(%q, %d)=%d want=%d", tt.in, tt.def, got, tt.want)
			}
		})
	}
}

func TestNullStr(t *testing.T) {
	if got := nullStr(""); got != nil {
		t.Fatal("nullStr(\"\") should return nil")
	}

	v := "127.0.0.1"
	got := nullStr(v)
	if got == nil {
		t.Fatal("nullStr(non-empty) should return pointer")
	}
	if *got != v {
		t.Fatalf("nullStr value=%q want=%q", *got, v)
	}
}
