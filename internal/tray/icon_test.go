package tray

import (
	"crypto/sha256"
	"image"
	"testing"
)

func hashImage(img image.Image) [32]byte {
	rgba := img.(*image.RGBA)
	return sha256.Sum256(rgba.Pix)
}

func TestProfileIconDiffers(t *testing.T) {
	quiet := hashImage(profileIcon("quiet"))
	custom := hashImage(profileIcon("custom"))
	balanced := hashImage(profileIcon("balanced"))
	performance := hashImage(profileIcon("performance"))

	if quiet == custom || quiet == balanced || quiet == performance || custom == balanced {
		t.Fatalf("profile icons are not distinct: quiet=%x custom=%x balanced=%x performance=%x",
			quiet, custom, balanced, performance)
	}
}

func TestProfileIconNotEmpty(t *testing.T) {
	img := profileIcon("quiet").(*image.RGBA)
	nonZero := 0
	for _, b := range img.Pix {
		if b != 0 {
			nonZero++
		}
	}
	if nonZero == 0 {
		t.Fatal("icon is fully transparent")
	}
}
