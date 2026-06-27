DELETE FROM system_settings WHERE key IN (
	'device_trust_enabled',
	'device_trust_allowed_os',
	'device_trust_min_os_version_mac',
	'device_trust_min_os_version_win',
	'device_trust_allowed_browsers',
	'device_trust_min_browser_version_chrome',
	'device_trust_min_browser_version_safari',
	'device_trust_min_browser_version_firefox',
	'device_trust_min_browser_version_edge',
	'device_trust_block_mobile'
);
