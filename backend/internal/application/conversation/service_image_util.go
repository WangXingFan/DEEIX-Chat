package conversation

import (
	"bytes"

	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/pkg/imageutil"
)

// resizeImageIfNeeded 按最大边长缩放图片，返回规整后的字节与实际 MIME。
func resizeImageIfNeeded(data []byte, mimeType string, maxDim int) ([]byte, string) {
	return imageutil.ResizeIfNeeded(data, mimeType, maxDim)
}

// resolveImageMimeType 归一化图片 MIME 类型，未知类型回退为 image/jpeg。
func resolveImageMimeType(mimeType string) string {
	return imageutil.ResolveMimeType(mimeType)
}

// sniffImageMIME 依据文件头识别受支持的图片类型，识别不出时返回空串。
func sniffImageMIME(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	if len(data) >= 8 && bytes.Equal(data[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) {
		return "image/png"
	}
	if len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff {
		return "image/jpeg"
	}
	if len(data) >= 12 && bytes.Equal(data[:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")) {
		return "image/webp"
	}
	if len(data) >= 6 && (bytes.Equal(data[:6], []byte("GIF87a")) || bytes.Equal(data[:6], []byte("GIF89a"))) {
		return "image/gif"
	}
	return ""
}
