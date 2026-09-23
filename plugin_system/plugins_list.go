package plugin_system

import (
	"goxcms/plugins/latest_posts_plugin"
	"goxcms/plugins/logger_plugin"
	"goxcms/plugins/shop_plugin"
)

func PluginList() []Plugin {
	plugin_list := []Plugin{

		&latest_posts_plugin.LatestPostsPlugin{},
		// &logger_plugin.LoggerPlugin{}, // uncomment to make the request logger available
		&shop_plugin.ShopPlugin{},

		/// add plugins here
		// comment to disable plugin
		// &your_plugin.YourPlugin{},

	}
	return plugin_list
}

// Compile-time check that the optional logger plugin satisfies Plugin.
var _ Plugin = (*logger_plugin.LoggerPlugin)(nil)
