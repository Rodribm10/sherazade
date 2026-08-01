package handler

import "testing"

func TestSupportAttachmentContentType(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		sample   []byte
		allowed  bool
	}{
		{name: "png", filename: "print.png", sample: []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, allowed: true},
		{name: "jpeg", filename: "foto.jpg", sample: []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F'}, allowed: true},
		{name: "webp", filename: "print.webp", sample: []byte{'R', 'I', 'F', 'F', 0x04, 0x00, 0x00, 0x00, 'W', 'E', 'B', 'P'}, allowed: true},
		{name: "pdf", filename: "relato.pdf", sample: []byte("%PDF-1.7\n"), allowed: true},
		{name: "extensao falsa", filename: "ata.pdf", sample: []byte("texto comum"), allowed: false},
		{name: "svg bloqueado", filename: "payload.svg", sample: []byte(`<svg onload="alert(1)"></svg>`), allowed: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, allowed := supportAttachmentContentType(test.filename, test.sample)
			if allowed != test.allowed {
				t.Fatalf("allowed=%v, want %v", allowed, test.allowed)
			}
		})
	}
}
