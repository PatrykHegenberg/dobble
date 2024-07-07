package main

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log/slog"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"

	"github.com/charmbracelet/huh"
	"github.com/disintegration/imaging"
	"github.com/go-pdf/fpdf"
)

const (
	imgDir         = "./img"
	cardWidth      = 55.0
	cardHeight     = 85.0
	margin         = 5.0
	dpiScale       = 3.779528 // 96 DPI
	outputFileName = "dobble_cards.pdf"
	minScaleFactor = 0.7
	maxScaleFactor = 1.0
)

type CardGenerator struct {
	TotalCards    int
	ImagesPerCard int
	ImageFiles    []string
	RoundCards    bool
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cg, err := getInputAndInitialize()
	if err != nil {
		logger.Error("Initialization failed", "error", err)
		os.Exit(1)
	}

	cards := cg.generateCards()
	logger.Info("Cards generated", "count", len(cards))

	if err := generatePDF(cards, cg.RoundCards); err != nil {
		logger.Error("PDF generation failed", "error", err)
		os.Exit(1)
	}

	logger.Info("PDF successfully generated")
}

func getInputAndInitialize() (*CardGenerator, error) {
	var totalCardsStr, imagesPerCardStr string
	var roundCards bool

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title("Enter the total number of cards:").Value(&totalCardsStr),
			huh.NewInput().Title("Enter the number of images per card:").Value(&imagesPerCardStr),
			huh.NewConfirm().
				Title("Do you want round cards?").
				Value(&roundCards),
		),
	)

	if err := form.Run(); err != nil {
		return nil, fmt.Errorf("form input failed: %w", err)
	}

	totalCards, err1 := strconv.Atoi(totalCardsStr)
	imagesPerCard, err2 := strconv.Atoi(imagesPerCardStr)
	if err := errors.Join(err1, err2); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	cg := &CardGenerator{
		TotalCards:    totalCards,
		ImagesPerCard: imagesPerCard,
		RoundCards:    roundCards,
	}

	if err := cg.loadImageFiles(); err != nil {
		return nil, err
	}

	return cg, nil
}

func (cg *CardGenerator) generateCards() [][]string {
	n := cg.ImagesPerCard - 1
	totalCards := n*n + n + 1

	if len(cg.ImageFiles) < totalCards {
		slog.Error("Not enough images for the given parameters",
			"required", totalCards,
			"available", len(cg.ImageFiles))
		return nil
	}

	cards := cg.generateCardIndices(n)
	imageCards := cg.convertToImageCards(cards)
	cg.shuffleCards(imageCards)

	return cg.limitCards(imageCards)
}

func (cg *CardGenerator) generateCardIndices(n int) [][]int {
	cards := make([][]int, 0, n*n+n+1)

	for i := 0; i < n+1; i++ {
		card := make([]int, cg.ImagesPerCard)
		card[0] = 1
		for j := 0; j < n; j++ {
			card[j+1] = (j + 1) + (i * n) + 1
		}
		cards = append(cards, card)
	}

	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			card := make([]int, cg.ImagesPerCard)
			card[0] = i + 2
			for k := 0; k < n; k++ {
				card[k+1] = (n + 1 + n*k + (i*k+j)%n) + 1
			}
			cards = append(cards, card)
		}
	}

	return cards
}

func (cg *CardGenerator) convertToImageCards(cards [][]int) [][]string {
	imageCards := make([][]string, len(cards))
	for i, card := range cards {
		imageCards[i] = make([]string, len(card))
		for j, symbolIndex := range card {
			imageCards[i][j] = cg.ImageFiles[symbolIndex-1]
		}
	}
	return imageCards
}

func (cg *CardGenerator) shuffleCards(cards [][]string) {
	rand.Shuffle(len(cards), func(i, j int) {
		cards[i], cards[j] = cards[j], cards[i]
	})

	for i := range cards {
		rand.Shuffle(len(cards[i]), func(j, k int) {
			cards[i][j], cards[i][k] = cards[i][k], cards[i][j]
		})
	}
}

func (cg *CardGenerator) limitCards(cards [][]string) [][]string {
	if cg.TotalCards < len(cards) {
		return cards[:cg.TotalCards]
	}
	return cards
}

func (cg *CardGenerator) loadImageFiles() error {
	files, err := os.ReadDir(imgDir)
	if err != nil {
		return fmt.Errorf("failed to read image directory: %w", err)
	}

	for _, file := range files {
		if !file.IsDir() && filepath.Ext(file.Name()) == ".png" {
			cg.ImageFiles = append(cg.ImageFiles, filepath.Join(imgDir, file.Name()))
		}
	}

	requiredImages := cg.calculateRequiredImages()

	if len(cg.ImageFiles) < requiredImages {
		return fmt.Errorf("not enough images in the img folder: required %d, found %d", requiredImages, len(cg.ImageFiles))
	}

	rand.Shuffle(len(cg.ImageFiles), func(i, j int) {
		cg.ImageFiles[i], cg.ImageFiles[j] = cg.ImageFiles[j], cg.ImageFiles[i]
	})

	return nil
}

func (cg *CardGenerator) calculateRequiredImages() int {
	n := cg.ImagesPerCard - 1
	return n*n + n + 1
}

func generatePDF(cards [][]string, roundCards bool) error {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetAutoPageBreak(true, 10)

	pageWidth, pageHeight, _ := pdf.PageSize(1)
	cardSize := math.Min(cardWidth, cardHeight)
	cardsPerRow := int((pageWidth - 2*margin) / (cardSize + margin))
	cardsPerCol := int((pageHeight - 2*margin) / (cardSize + margin))
	cardsPerPage := cardsPerRow * cardsPerCol

	for i, card := range cards {
		if i%cardsPerPage == 0 {
			pdf.AddPage()
		}

		col := i % cardsPerRow
		row := (i / cardsPerRow) % cardsPerCol

		x := margin + float64(col)*(cardSize+margin)
		y := margin + float64(row)*(cardSize+margin)

		slog.Info("Processing card", "index", i, "x", x, "y", y)

		if roundCards {
			if err := processRoundCard(pdf, x, y, card); err != nil {
				return fmt.Errorf("failed to process round card %d: %w", i, err)
			}
		} else {
			if err := processSquareCard(pdf, x, y, card); err != nil {
				return fmt.Errorf("failed to process square card %d: %w", i, err)
			}
		}
	}

	return pdf.OutputFileAndClose(outputFileName)
}

func processRoundCard(pdf *fpdf.Fpdf, x, y float64, card []string) error {
	diameter := math.Min(cardWidth, cardHeight)
	radius := diameter / 2

	pdf.SetDrawColor(0, 0, 0)
	pdf.Circle(x+radius, y+radius, radius, "D")

	availableRadius := radius - 5
	optimalImageSize := availableRadius * 2 / math.Sqrt(float64(len(card)))

	for i, imgFile := range card {
		angle := 2 * math.Pi * float64(i) / float64(len(card))
		distanceFromCenter := availableRadius * 0.6

		imgX := x + radius + distanceFromCenter*math.Cos(angle) - optimalImageSize/2
		imgY := y + radius + distanceFromCenter*math.Sin(angle) - optimalImageSize/2

		if err := processImage(pdf, imgFile, imgX, imgY, optimalImageSize); err != nil {
			return err
		}
	}

	return nil
}

func processSquareCard(pdf *fpdf.Fpdf, x, y float64, card []string) error {
	pdf.Rect(x, y, cardWidth, cardHeight, "D")

	availableWidth := cardWidth - 10
	availableHeight := cardHeight - 10
	optimalImageSize := math.Min(availableWidth/2, availableHeight/float64(len(card)))

	for i, imgFile := range card {
		imgX := x + 5 + rand.Float64()*(availableWidth-optimalImageSize)
		imgY := y + 5 + float64(i)*(availableHeight/float64(len(card))) + rand.Float64()*(availableHeight/float64(len(card))-optimalImageSize)

		if err := processImage(pdf, imgFile, imgX, imgY, optimalImageSize); err != nil {
			return err
		}
	}

	return nil
}

func processImage(pdf *fpdf.Fpdf, imgFile string, x, y, size float64) error {
	file, err := os.Open(imgFile)
	if err != nil {
		return fmt.Errorf("failed to open image file: %w", err)
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return fmt.Errorf("failed to decode image: %w", err)
	}

	scaleFactor := minScaleFactor + rand.Float64()*(maxScaleFactor-minScaleFactor)
	imgSize := size * scaleFactor
	targetSize := uint(imgSize * dpiScale)

	img = imaging.Fit(img, int(targetSize), int(targetSize), imaging.Lanczos)

	rotation := rand.Intn(4) * 90
	rotatedImg := imaging.Rotate(img, float64(rotation), color.Transparent)

	tmpFile, err := os.CreateTemp("", "processed_*.png")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if err := png.Encode(tmpFile, rotatedImg); err != nil {
		return fmt.Errorf("failed to encode processed image: %w", err)
	}
	tmpFile.Close()

	pdf.ImageOptions(
		tmpFile.Name(),
		x, y,
		imgSize, imgSize,
		false,
		fpdf.ImageOptions{ImageType: "PNG"},
		0,
		"",
	)

	return nil
}
