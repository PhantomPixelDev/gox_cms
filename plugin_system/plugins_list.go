package plugin_system

import (
	"goxcms/plugins/latest_posts_plugin"
	"goxcms/plugins/logger_plugin"
)

func PluginList() []Plugin {
	plugin_list := []Plugin{

		&latest_posts_plugin.LatestPostsPlugin{},
		&logger_plugin.LoggerPlugin{}, // add your pl
		// Add your pl

		/// add plugins here
		// comment to disable plugin
		// &your_plugin.YourPlugin{},

	}
	return plugin_list
}
