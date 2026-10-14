package filetype

import "testing"

func TestIsText(t *testing.T) {
	tests := []struct {
		mimeType string
		fileName string
		want     bool
	}{
		{mimeType: "text/plain", fileName: "file.bin", want: true},
		{mimeType: "application/json", fileName: "file.bin", want: true},
		{mimeType: "application/octet-stream", fileName: "README.MD", want: true},
		{mimeType: "image/png", fileName: "image.png", want: false},
	}
	for _, test := range tests {
		if got := IsText(test.mimeType, test.fileName); got != test.want {
			t.Fatalf("IsText(%q, %q) = %v, want %v", test.mimeType, test.fileName, got, test.want)
		}
	}
}
