package plugin_system

import (
	"goxcms/plugins/latest_posts_plugin"
	"goxcms/plugins/shop_plugin"
)

func PluginList() []Plugin {
	plugin_list := []Plugin{

		&latest_posts_plugin.LatestPostsPlugin{},
		//&logger_plugin.LoggerPlugin{},
		&shop_plugin.ShopPlugin{},

		/// add plugins here
		// comment to disable plugin
		// &your_plugin.YourPlugin{},

	}
	return plugin_list
}
