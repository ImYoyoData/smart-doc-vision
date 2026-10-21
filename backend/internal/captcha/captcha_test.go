package captcha

import (
	"bytes"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestGenerateChallenge(t *testing.T) {
	challenge, answer, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	if answer < padding || answer > Width-PieceSize-padding {
		t.Fatalf("answer out of bounds: %d", answer)
	}
	if challenge.PieceY < 30 || challenge.PieceY > Height-PieceSize-24 {
		t.Fatalf("piece Y out of bounds: %d", challenge.PieceY)
	}
	if _, err := jpeg.Decode(bytes.NewReader(challenge.Background)); err != nil {
		t.Fatalf("background is not valid JPEG: %v", err)
	}
	piece, err := png.Decode(bytes.NewReader(challenge.Piece))
	if err != nil {
		t.Fatalf("piece is not valid PNG: %v", err)
	}
	if piece.Bounds().Dx() != PieceSize || piece.Bounds().Dy() != PieceSize {
		t.Fatalf("unexpected piece size: %v", piece.Bounds())
	}
}

func TestInsidePiece(t *testing.T) {
	if !insidePiece(PieceSize/2, PieceSize/2, PieceSize) {
		t.Fatal("center should be inside the piece")
	}
	if insidePiece(-1, 2, PieceSize) || insidePiece(PieceSize, 2, PieceSize) {
		t.Fatal("outside coordinates must not be inside the piece")
	}
}
