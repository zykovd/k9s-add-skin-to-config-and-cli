package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/derailed/k9s/internal/config"
	"github.com/derailed/k9s/internal/render"
	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog/log"
)

// Synchronizer manages ui event queue.
type synchronizer interface {
	QueueUpdateDraw(func())
	QueueUpdate(func())
}

// Configurator represents an application configuration.
type Configurator struct {
	Config     *config.Config
	Styles     *config.Styles
	CustomView *config.CustomView
	BenchFile  string
	skinFile   string
}

// HasSkin returns true if a skin file was located.
func (c *Configurator) HasSkin() bool {
	return c.skinFile != ""
}

// CustomViewsWatcher watches for view config file changes.
func (c *Configurator) CustomViewsWatcher(ctx context.Context, s synchronizer) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	go func() {
		for {
			select {
			case evt := <-w.Events:
				_ = evt
				s.QueueUpdateDraw(func() {
					c.RefreshCustomViews()
				})
			case err := <-w.Errors:
				log.Warn().Err(err).Msg("CustomView watcher failed")
				return
			case <-ctx.Done():
				log.Debug().Msgf("CustomViewWatcher CANCELED `%s!!", config.K9sViewConfigFile)
				if err := w.Close(); err != nil {
					log.Error().Err(err).Msg("Closing CustomView watcher")
				}
				return
			}
		}
	}()

	log.Debug().Msgf("CustomView watching `%s", config.K9sViewConfigFile)
	c.RefreshCustomViews()
	return w.Add(config.K9sViewConfigFile)
}

// RefreshCustomViews load view configuration changes.
func (c *Configurator) RefreshCustomViews() {
	if c.CustomView == nil {
		c.CustomView = config.NewCustomView()
	} else {
		c.CustomView.Reset()
	}

	if err := c.CustomView.Load(config.K9sViewConfigFile); err != nil {
		log.Warn().Err(err).Msgf("Custom view load failed %s", config.K9sViewConfigFile)
		return
	}
}

// StylesWatcher watches for skin file changes.
func (c *Configurator) StylesWatcher(ctx context.Context, s synchronizer) error {
	if !c.HasSkin() {
		return nil
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	go func() {
		for {
			select {
			case evt := <-w.Events:
				if evt.Op != fsnotify.Chmod {
					s.QueueUpdateDraw(func() {
						c.RefreshStyles(
							c.Config.K9s.CurrentCluster,
							c.Config.K9s.Skin,
							c.Config.K9s.GetManualSkin(),
						)
					})
				}
			case err := <-w.Errors:
				log.Info().Err(err).Msg("Skin watcher failed")
				return
			case <-ctx.Done():
				log.Debug().Msgf("SkinWatcher CANCELED `%s!!", c.skinFile)
				if err := w.Close(); err != nil {
					log.Error().Err(err).Msg("Closing Skin watcher")
				}
				return
			}
		}
	}()

	log.Debug().Msgf("SkinWatcher watching `%s", c.skinFile)
	return w.Add(c.skinFile)
}

// BenchConfig location of the benchmarks configuration file.
func BenchConfig(context string) string {
	return filepath.Join(config.K9sHome(), config.K9sBench+"-"+context+".yml")
}

// RefreshStyles load for skin configuration changes.
func (c *Configurator) RefreshStyles(context string, configuredStyle string, manualStyle string) {
	c.BenchFile = BenchConfig(context)

	if c.Styles == nil {
		c.Styles = config.NewStyles()
	} else {
		c.Styles.Reset()
	}

	// Check skin from cli args
	if manualStyle != "" && c.updateStylesWithFile(manualStyle) {
		return
	}

	// Check context specific skin files
	if context != "" && c.updateStylesWithFile(fmt.Sprintf("%s_skin", context)) {
		return
	}

	// Check prefered skin set from k9s config
	if configuredStyle != "" && c.updateStylesWithFile(configuredStyle) {
		return
	}

	// Check default files (skin.yml/skin.yaml)
	if c.updateStylesWithFile(config.K9sDefaultSkin) {
		return
	}

	c.updateStyles("")
}

func (c *Configurator) updateStylesWithFile(skinName string) bool {
	for _, extension := range config.K9sStylesFileExtensions {
		manualSkin := filepath.Join(config.K9sHome(), fmt.Sprintf("%s.%s", skinName, extension))
		if c.loadSkinFile(manualSkin) {
			return true
		}
	}
	return false
}

func (c *Configurator) loadSkinFile(fileName string) bool {
	if err := c.Styles.Load(fileName); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			log.Warn().Msgf("No skin file found -- %s", fileName)
		} else {
			log.Error().Msgf("Failed to parse skin file -- %s. %s.", fileName, err)
		}
		return false
	} else {
		c.updateStyles(fileName)
		return true
	}
}

func (c *Configurator) updateStyles(f string) {
	c.skinFile = f
	if !c.HasSkin() {
		c.Styles.DefaultSkin()
	}
	c.Styles.Update()

	render.ModColor = c.Styles.Frame().Status.ModifyColor.Color()
	render.AddColor = c.Styles.Frame().Status.AddColor.Color()
	render.ErrColor = c.Styles.Frame().Status.ErrorColor.Color()
	render.StdColor = c.Styles.Frame().Status.NewColor.Color()
	render.PendingColor = c.Styles.Frame().Status.PendingColor.Color()
	render.HighlightColor = c.Styles.Frame().Status.HighlightColor.Color()
	render.KillColor = c.Styles.Frame().Status.KillColor.Color()
	render.CompletedColor = c.Styles.Frame().Status.CompletedColor.Color()
}
