package demo

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"

	"injectctl/internal/config"
	"injectctl/internal/core"
)

var bitmapFont = map[rune][]string{
	'-': {
		"00000",
		"00000",
		"00000",
		"11111",
		"00000",
		"00000",
		"00000",
	},
	'.': {
		"00000",
		"00000",
		"00000",
		"00000",
		"00000",
		"01100",
		"01100",
	},
	'0': {
		"01110",
		"10001",
		"10011",
		"10101",
		"11001",
		"10001",
		"01110",
	},
	'1': {
		"00100",
		"01100",
		"00100",
		"00100",
		"00100",
		"00100",
		"01110",
	},
	'2': {
		"01110",
		"10001",
		"00001",
		"00010",
		"00100",
		"01000",
		"11111",
	},
	'5': {
		"11111",
		"10000",
		"11110",
		"00001",
		"00001",
		"10001",
		"01110",
	},
	'8': {
		"01110",
		"10001",
		"10001",
		"01110",
		"10001",
		"10001",
		"01110",
	},
	':': {
		"00000",
		"01100",
		"01100",
		"00000",
		"01100",
		"01100",
		"00000",
	},
	'/': {
		"00001",
		"00010",
		"00100",
		"01000",
		"10000",
		"00000",
		"00000",
	},
	'H': {
		"10001",
		"10001",
		"11111",
		"10001",
		"10001",
		"10001",
		"10001",
	},
	'O': {
		"01110",
		"10001",
		"10001",
		"10001",
		"10001",
		"10001",
		"01110",
	},
	'P': {
		"11110",
		"10001",
		"10001",
		"11110",
		"10000",
		"10000",
		"10000",
	},
	'S': {
		"01111",
		"10000",
		"10000",
		"01110",
		"00001",
		"00001",
		"11110",
	},
	'T': {
		"11111",
		"00100",
		"00100",
		"00100",
		"00100",
		"00100",
		"00100",
	},
	'X': {
		"10001",
		"10001",
		"01010",
		"00100",
		"01010",
		"10001",
		"10001",
	},
	'c': {
		"00000",
		"00000",
		"01110",
		"10000",
		"10000",
		"10001",
		"01110",
	},
	'h': {
		"10000",
		"10000",
		"10110",
		"11001",
		"10001",
		"10001",
		"10001",
	},
	'm': {
		"00000",
		"00000",
		"11010",
		"10101",
		"10101",
		"10101",
		"10101",
	},
	'n': {
		"00000",
		"00000",
		"10110",
		"11001",
		"10001",
		"10001",
		"10001",
	},
	'o': {
		"00000",
		"00000",
		"01110",
		"10001",
		"10001",
		"10001",
		"01110",
	},
	'p': {
		"00000",
		"00000",
		"11110",
		"10001",
		"11110",
		"10000",
		"10000",
	},
	's': {
		"00000",
		"00000",
		"01111",
		"10000",
		"01110",
		"00001",
		"11110",
	},
	't': {
		"00100",
		"00100",
		"11111",
		"00100",
		"00100",
		"00101",
		"00010",
	},
	'u': {
		"00000",
		"00000",
		"10001",
		"10001",
		"10001",
		"10011",
		"01101",
	},
	' ': {
		"00000",
		"00000",
		"00000",
		"00000",
		"00000",
		"00000",
		"00000",
	},
}

func Generate(root string, mode core.Mode) error {
	artifactDir := filepath.Join(root, "artifacts")
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(artifactDir, "notes.txt"), []byte(demoNotes()), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(artifactDir, "scan.nmap"), []byte(demoNmap()), 0o644); err != nil {
		return err
	}
	if err := writeScreenshot(filepath.Join(artifactDir, "terminal.png"), []string{
		"nmap",
		"22/tcp",
		"80/tcp",
		"SSH HTTP",
		"10.0.14.5",
	}); err != nil {
		return err
	}

	cfg := config.DefaultConfig()
	cfg.Mode = mode
	cfg.Client = "Example Corp"
	cfg.Environment = "Production"
	cfg.Artifacts = []string{"./artifacts"}
	cfg.Template = "./templates/default/" + string(mode) + ".md.tmpl"
	cfg.Output.ProjectDir = "./project"
	switch mode {
	case core.ModeAssess:
		cfg.Title = "Alpha Demo Assessment"
		cfg.Instructions = "Turn the included demo artifacts into a draft corporate assessment report."
	case core.ModeInject:
		cfg.Title = "Alpha Demo Inject"
		cfg.Instructions = "Turn the included demo artifacts into a draft exercise inject pack."
	}

	data, err := config.MarshalYAML(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, "job.yaml"), data, 0o644)
}

func demoNotes() string {
	return "Analyst note: SSH and HTTP are visible on a public-facing host. Review service exposure and expected hardening baseline.\n"
}

func demoNmap() string {
	return "Nmap scan report for demo target\n22/tcp open ssh OpenSSH 8.2p1 Ubuntu 4ubuntu0.11\n80/tcp open http Apache httpd 2.4.41 ((Ubuntu))\n"
}

func writeScreenshot(path string, lines []string) error {
	const (
		scale       = 12
		glyphWidth  = 5
		glyphHeight = 7
		padding     = 24
		lineGap     = 14
	)

	maxRunes := 0
	for _, line := range lines {
		if len([]rune(line)) > maxRunes {
			maxRunes = len([]rune(line))
		}
	}
	width := padding*2 + maxRunes*(glyphWidth*scale+scale)
	height := padding*2 + len(lines)*(glyphHeight*scale) + max(0, len(lines)-1)*lineGap
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	fillImage(img, color.RGBA{R: 248, G: 250, B: 252, A: 255})

	for lineIdx, line := range lines {
		y := padding + lineIdx*(glyphHeight*scale+lineGap)
		x := padding
		for _, char := range line {
			drawGlyph(img, x, y, char, scale)
			x += glyphWidth*scale + scale
		}
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	return png.Encode(file, img)
}

func fillImage(img *image.RGBA, fill color.Color) {
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			img.Set(x, y, fill)
		}
	}
}

func drawGlyph(img *image.RGBA, startX, startY int, char rune, scale int) {
	pattern, ok := bitmapFont[char]
	if !ok {
		pattern = bitmapFont[' ']
	}
	for row, line := range pattern {
		for col, pixel := range line {
			if pixel != '1' {
				continue
			}
			for dy := 0; dy < scale; dy++ {
				for dx := 0; dx < scale; dx++ {
					img.Set(startX+col*scale+dx, startY+row*scale+dy, color.Black)
				}
			}
		}
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
