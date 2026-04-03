/*
 * Spinner - 加载动画
 */

package terminal

import (
	"fmt"
	"strings"
)

// Spinner 加载动画
type Spinner struct {
	frames     []string
	frameIndex int
	theme      ColorTheme
}

// NewSpinner 创建加载动画
func NewSpinner() *Spinner {
	return &Spinner{
		frames: []string{
			"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏",
		},
		frameIndex: 0,
		theme:      DefaultTheme(),
	}
}

// Tick 显示下一帧
func (s *Spinner) Tick(label string) {
	frame := s.frames[s.frameIndex%s.frameCount()]
	s.frameIndex++
	fmt.Printf("\r%s %s%s%s", s.theme.Spinner, frame, s.theme.Reset(), " "+label)
}

// Stop 停止动画
func (s *Spinner) Stop(label string, success bool) {
	s.frameIndex = 0
	if success {
		fmt.Printf("\r%s ✔ %s%s\n", s.theme.SpinnerDone, label, s.theme.Reset())
	} else {
		fmt.Printf("\r%s ✘ %s%s\n", s.theme.SpinnerFail, label, s.theme.Reset())
	}
}

func (s *Spinner) frameCount() int {
	return len(s.frames)
}

// ProgressBar 进度条
type ProgressBar struct {
	theme    ColorTheme
	total    int
	current  int
	width    int
	showPercent bool
}

// NewProgressBar 创建进度条
func NewProgressBar(total int) *ProgressBar {
	return &ProgressBar{
		theme:    DefaultTheme(),
		total:   total,
		current: 0,
		width:   40,
		showPercent: true,
	}
}

// SetProgress 设置进度
func (p *ProgressBar) SetProgress(current int) {
	p.current = current
}

// Increment 递增进度
func (p *ProgressBar) Increment() {
	p.current++
}

// Render 渲染进度条
func (p *ProgressBar) Render(label string) {
	if p.total == 0 {
		return
	}

	percent := float64(p.current) / float64(p.total)
	filled := int(float64(p.width) * percent)

	bar := strings.Repeat("█", filled) + strings.Repeat("░", p.width-filled)

	fmt.Printf("\r%s [%s] %d/%d", label, bar, p.current, p.total)
	if p.showPercent {
		fmt.Printf(" (%.0f%%)", percent*100)
	}
}

// Clear 清除进度条
func (p *ProgressBar) Clear() {
	fmt.Printf("\r%s\r", strings.Repeat(" ", p.width+50))
}

// Done 完成
func (p *ProgressBar) Done(label string) {
	p.Clear()
	percent := 100
	filled := p.width
	bar := strings.Repeat("█", filled)
	fmt.Printf("%s ✔ %s [%s] %d/%d (%.0f%%)%s\n",
		p.theme.SpinnerDone, label, bar, p.total, p.total, float64(percent), p.theme.Reset())
}
