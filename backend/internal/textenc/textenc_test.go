package textenc

import (
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

func TestNormalizeUTF8BOM(t *testing.T) {
	if got := Normalize("\uFEFF合同协议"); got != "合同协议" {
		t.Fatalf("unexpected BOM normalization: %q", got)
	}
}

func TestNormalizeGB18030(t *testing.T) {
	encoded, _, err := transform.Bytes(simplifiedchinese.GB18030.NewEncoder(), []byte("工程图纸审核"))
	if err != nil {
		t.Fatal(err)
	}
	if got := Normalize(string(encoded)); got != "工程图纸审核" {
		t.Fatalf("unexpected GB18030 normalization: %q", got)
	}
}

func TestNormalizeLatin1Mojibake(t *testing.T) {
	encoded, _, err := transform.Bytes(simplifiedchinese.GB18030.NewEncoder(), []byte("合同协议"))
	if err != nil {
		t.Fatal(err)
	}
	mojibake := make([]rune, 0, len(encoded))
	for _, b := range encoded {
		mojibake = append(mojibake, rune(b))
	}
	if got := Normalize(string(mojibake)); got != "合同协议" {
		t.Fatalf("unexpected mojibake repair: %q", got)
	}
}

func TestNormalizeKeepsUTF8(t *testing.T) {
	const text = "中文 content with English\n第二行"
	if got := Normalize(text); got != text {
		t.Fatalf("unexpected UTF-8 change: %q", got)
	}
}
