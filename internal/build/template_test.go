package build

import "testing"

func TestSafeNamePortableComponent(t *testing.T) {
	tests := map[string]string{
		"normal-name_1.2": "normal-name_1.2",
		"hello world/世界":  "hello_world___",
		"":                "_", ".": "_", "..": "_",
		"name.": "name_", "name..": "name__", "CON": "_CON", "con.txt": "_con.txt", "CON.": "_CON_",
		"PRN.log": "_PRN.log", "AUX": "_AUX", "NUL.bin": "_NUL.bin",
		"COM1": "_COM1", "com9.txt": "_com9.txt", "LPT1": "_LPT1", "lpt9.x": "_lpt9.x",
		"COM0": "COM0", "COM10": "COM10", "LPT0": "LPT0",
	}
	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			if got := safeName(input); got != want {
				t.Fatalf("safeName(%q) = %q, want %q", input, got, want)
			}
		})
	}
}
