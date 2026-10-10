package github.shadowbaby.clawproxyhub.core

import android.net.Uri
import android.graphics.BitmapFactory
import android.os.PowerManager
import android.view.View
import android.view.ViewGroup
import android.webkit.WebView
import android.widget.FrameLayout
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.detectHorizontalDragGestures
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.selection.selectable
import androidx.compose.ui.semantics.Role
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.runtime.*
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.toArgb
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.unit.IntOffset
import kotlin.math.roundToInt
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import androidx.compose.ui.draw.clip
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.ComposeView
import androidx.compose.ui.platform.ViewCompositionStrategy
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.vectorResource
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.semantics.testTagsAsResourceId
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.viewinterop.AndroidView
import org.json.JSONArray
import org.json.JSONObject
import top.yukonga.miuix.kmp.basic.*
import top.yukonga.miuix.kmp.preference.SwitchPreference
import top.yukonga.miuix.kmp.preference.OverlaySpinnerPreference
import top.yukonga.miuix.kmp.theme.MiuixTheme
import top.yukonga.miuix.kmp.theme.darkColorScheme
import top.yukonga.miuix.kmp.theme.lightColorScheme
import top.yukonga.miuix.kmp.window.WindowDialog

// 应用级交互使用 Miuix；运行时通过核心私有入口复用安装生命周期。
internal class NativePages(private val app: MainActivity) {
    private var tab by mutableIntStateOf(1)
    private var web by mutableStateOf<WebView?>(null)
    private var workspaceReady by mutableStateOf(false)
    private var installed by mutableStateOf<List<JSONObject>>(emptyList())
    private var pluginExtensions by mutableStateOf(JSONArray())
    private var luaRuntime by mutableStateOf("runtime_loading")
    private var readingPlugins by mutableStateOf(false)
    private var marketItems by mutableStateOf<List<JSONObject>>(emptyList())
    private var marketOnline by mutableStateOf(false)
    private var marketLoading by mutableStateOf(false)
    private var pluginTab by mutableIntStateOf(0)
    private var installingName by mutableStateOf("")
    private var operationName by mutableStateOf("")
    private var operationUnread by mutableStateOf(false)
    private var installPhase by mutableStateOf("")
    private var receivedBytes by mutableLongStateOf(0)
    private var totalBytes by mutableLongStateOf(-1)
    private var installError by mutableStateOf("")
    private val installLog = mutableStateListOf<String>()
    private var installStarted = 0L
    private var loggedPhase = ""
    private var loggedBucket = -1L
    private var floatingTabs by mutableStateOf(AppPreferences.floatingTabs(app))
    private var keyboardVisible by mutableStateOf(false)
    private var logging by mutableStateOf(AppLog.enabled(app))
    private var logLevel by mutableStateOf(AppLog.level(app))
    private var logDays by mutableIntStateOf(AppLog.days(app))
    private var logContent by mutableStateOf<String?>(null)
    private var settingsPage by mutableStateOf("")
    private var runtimeState by mutableStateOf<JSONObject?>(null)
    private var runtimePackages by mutableStateOf<List<JSONObject>>(emptyList())
    private var runtimeError by mutableStateOf<String?>(null)
    private var readingRuntimes by mutableStateOf(false)
    private var runtimeSettings by mutableStateOf<JSONObject?>(null)
    private var runtimeSettingsError by mutableStateOf<String?>(null)
    private var runtimeSettingsBusy by mutableStateOf(false)
    private val runtimeValues = mutableStateMapOf<String, Any>()
    private var latestRuntime by mutableStateOf<JSONObject?>(null)
    private var runtimeUpdateChecked by mutableStateOf(false)
    private var runtimeUpdateLoading by mutableStateOf(false)
    private var runtimeUpdateError by mutableStateOf<String?>(null)
    private var githubProxy by mutableStateOf(AppPreferences.githubProxy(app))
    private var systemNotifications by mutableStateOf(AppNotifications.system(app))
    private var notificationsAllowed by mutableStateOf(AppNotifications.available(app))
    private var update by mutableStateOf<JSONObject?>(null)
    private var updateError by mutableStateOf<String?>(null)
    private var locale by mutableStateOf(AppPreferences.locale(app))
    private var theme by mutableStateOf(AppPreferences.theme(app))
    private var remoteOnly by mutableStateOf(AppPreferences.remoteOnly(app))
    private var batteryExempt by mutableStateOf(false)
    private var dialog by mutableStateOf("")
    private var connections by mutableStateOf<List<JSONObject>>(emptyList())
    private var currentConnection by mutableStateOf("")
    private var dialogTitle by mutableStateOf("")
    private var dialogMessage by mutableStateOf("")
    private var confirmation: () -> Unit = {}
    private var luaSource: Uri? = null
    private var editingConnection by mutableStateOf<JSONObject?>(null)
    private var selectedPlugin by mutableStateOf<JSONObject?>(null)
    private var drawer by mutableStateOf(false)
    private var search by mutableStateOf("")

    fun text(zh: String, en: String) = if (locale == "en") en else zh
    fun keyboard(visible: Boolean) { keyboardVisible = visible }
    fun background() = if (theme == "dark") 0xff121212.toInt() else 0xfff7f7f7.toInt()
    fun create(): View = ComposeView(app).apply {
        tag = "native.shell"
        setViewCompositionStrategy(ViewCompositionStrategy.DisposeOnViewTreeLifecycleDestroyed)
        setContent { Shell() }
    }
    fun selectTab(value: Int) { if (tab != value) { drawer = false; settingsPage = "" }; tab = value }
    fun back(): Boolean { if (dialog.isNotEmpty()) { dismissDialog(); return true }; if (drawer) { drawer = false; return true }; if (settingsPage.isNotEmpty()) { settingsPage = if (settingsPage == "runtime-settings") "runtimes" else ""; return true }; return false }
    fun workspace(value: WebView?, ready: Boolean) { web = value; workspaceReady = ready }
    fun loadingPlugins() { readingPlugins = true }
    fun plugins(items: JSONArray) {
        installed = items.objects(); readingPlugins = false
        selectedPlugin?.let { selected -> selectedPlugin = installed.find { it.optString("name") == selected.optString("name") } }
    }
    fun pluginRuntime(status: String) { luaRuntime = status }
    fun pluginExtensions(states: JSONArray) { pluginExtensions = states }
    fun loadingMarket() { marketLoading = true }
    fun market(items: JSONArray, online: Boolean, loading: Boolean) { marketItems = items.objects(); marketOnline = online; marketLoading = loading }
    fun marketUnavailable() { marketLoading = false; marketOnline = false }
    fun installing(name: String) { installingName = name; operationName = name; operationUnread = true; installStarted = android.os.SystemClock.elapsedRealtime(); installLog.clear(); loggedPhase = ""; loggedBucket = -1; installError = ""; installProgress("waiting", 0, -1); dialog = "progress" }
    fun installProgress(phase: String, received: Long, total: Long) {
        installPhase = phase; receivedBytes = received; totalBytes = total
        val bucket = if (total > 0) received * 10 / total else received / (256 * 1024)
        if (phase != loggedPhase || phase == "downloading" && bucket != loggedBucket) { logInstall(operationText()); loggedPhase = phase; loggedBucket = bucket }
    }
    private fun logInstall(message: String) { installLog.add("${(android.os.SystemClock.elapsedRealtime() - installStarted) / 1000}s · $message"); if (installLog.size > 40) installLog.removeAt(0) }
    fun installComplete(name: String) { installingName = ""; operationName = name; installProgress("complete", 0, -1) }
    fun installFailed(message: String?) { installingName = ""; installError = message ?: text("请重试", "Please retry"); installProgress("failed", 0, -1); logInstall(installError); dialog = "progress" }
    private fun showInstallProgress() { dialog = "progress"; if (installingName.isEmpty()) operationUnread = false }
    private fun dismissDialog() { if (dialog == "progress" && installingName.isEmpty()) operationUnread = false; dialog = "" }
    fun openLogs() { logContent = null; settingsPage = "logs" }
    fun logs(content: String?) { logContent = content }
    fun openRuntimes() { settingsPage = "runtimes" }
    fun runtimesVisible() = settingsPage == "runtimes"
    fun runtimeSettingsVisible() = settingsPage == "runtime-settings"
    fun openRuntimeSettings() { settingsPage = "runtime-settings"; runtimeSettings = null; runtimeSettingsError = null; runtimeSettingsBusy = true }
    fun savingRuntimeSettings() { runtimeSettingsBusy = true; runtimeSettingsError = null }
    fun runtimeSettings(value: JSONObject?, error: String?) {
        if (!runtimeSettingsVisible()) return
        runtimeSettingsBusy = false; runtimeSettingsError = error
        if (value != null) {
            runtimeSettings = value; runtimeValues.clear()
            val values = value.getJSONObject("values")
            values.keys().forEach { runtimeValues[it] = values.get(it) }
        }
    }
    fun loadingRuntimes() { readingRuntimes = true }
    fun runtimes(value: JSONObject?, error: String?) {
        readingRuntimes = false; runtimeError = error
        if (value != null) {
            runtimeState = value.getJSONArray("installed").objects().find { it.optJSONObject("manifest")?.optString("id") == "lua-runtime" }
            runtimePackages = value.getJSONArray("packages").objects()
        }
    }
    fun checkingRuntimeUpdate() { runtimeUpdateLoading = true; runtimeUpdateError = null }
    fun runtimeUpdate(value: JSONObject?, error: String?) { latestRuntime = value; runtimeUpdateLoading = false; runtimeUpdateChecked = true; runtimeUpdateError = error }
    fun workspaceList(items: JSONArray, selected: String) { connections = items.objects(); currentConnection = selected }
    fun updateResult(value: JSONObject?, error: String?) { update = value; updateError = error; dialog = "update" }
    fun refreshPreferences() {
        locale = AppPreferences.locale(app); theme = AppPreferences.theme(app)
        remoteOnly = AppPreferences.remoteOnly(app)
        githubProxy = AppPreferences.githubProxy(app)
        systemNotifications = AppNotifications.system(app); notificationsAllowed = AppNotifications.available(app)
        floatingTabs = AppPreferences.floatingTabs(app); logging = AppLog.enabled(app); logLevel = AppLog.level(app); logDays = AppLog.days(app)
        batteryExempt = app.getSystemService(PowerManager::class.java).isIgnoringBatteryOptimizations(app.packageName)
    }
    fun chooseWorkspace(items: JSONArray, selected: String, remote: Boolean) {
        connections = items.objects(); currentConnection = selected; remoteOnly = remote; drawer = true
    }
    fun askLua(uri: Uri) { luaSource = uri; dialog = "lua" }
    fun error(message: String?) {
        dialogTitle = text("操作未完成", "Could not complete operation")
        dialogMessage = message ?: text("请重试。", "Please try again."); dialog = "error"
    }
    private fun confirm(title: String, message: String, action: () -> Unit) {
        dialogTitle = title; dialogMessage = message; confirmation = action; dialog = "confirm"
    }

    @Composable private fun Shell() {
        MiuixTheme(colors = if (theme == "dark") darkColorScheme() else lightColorScheme()) {
            val surface = MiuixTheme.colorScheme.surface.toArgb()
            SideEffect { WindowLayout.appearance(app, surface, theme != "dark") }
            Box(Modifier.fillMaxSize().semantics { testTagsAsResourceId = true }) {
            Scaffold(
                modifier = Modifier.fillMaxSize().semantics { testTagsAsResourceId = true },
                contentWindowInsets = WindowInsets(0, 0, 0, 0),
                topBar = { if (tab == 2 && settingsPage.isNotEmpty()) SettingsTopBar() },
                floatingActionButtonPosition = FabPosition.End,
                floatingActionButton = {
                    if (tab == 0) Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                        if (installingName.isNotEmpty() || operationUnread) FloatingActionButton(onClick = { showInstallProgress() },
                            modifier = Modifier.testTag("native.plugin-progress").semantics {
                                contentDescription = text("安装详情", "Installation details")
                                stateDescription = operationText()
                            }, containerColor = MiuixTheme.colorScheme.surfaceContainer, shadowElevation = 0.dp, minWidth = 48.dp, minHeight = 48.dp) {
                            if (installingName.isNotEmpty()) CircularProgressIndicator(modifier = Modifier.testTag("native.install-spinner"), size = 24.dp)
                            else Icon(ImageVector.vectorResource(R.drawable.action_details), null,
                                tint = if (installPhase == "failed") MiuixTheme.colorScheme.error else MiuixTheme.colorScheme.primary)
                        }
                        if (pluginTab == 0) IconButton(onClick = { app.refreshMarket() }, enabled = !marketLoading && installingName.isEmpty(),
                            modifier = Modifier.size(48.dp).testTag("native.refresh"), backgroundColor = MiuixTheme.colorScheme.surfaceContainer) {
                            Icon(ImageVector.vectorResource(R.drawable.action_refresh), text("刷新市场", "Refresh market"), tint = MiuixTheme.colorScheme.primary)
                        }
                        FloatingActionButton(onClick = { dialog = "import" }, modifier = Modifier.testTag("native.import"),
                            containerColor = MiuixTheme.colorScheme.surfaceContainer, shadowElevation = 0.dp, minWidth = 48.dp, minHeight = 48.dp) {
                            Icon(ImageVector.vectorResource(R.drawable.action_install), text("导入插件", "Import plugin"), tint = MiuixTheme.colorScheme.primary)
                        }
                    }
                },
                bottomBar = {
                    if (!keyboardVisible) {
                        if (floatingTabs) Box(Modifier.fillMaxWidth(), contentAlignment = Alignment.Center) {
                            FloatingToolbar(modifier = Modifier.testTag("native.tabs.floating"), outSidePadding = PaddingValues(horizontal = 24.dp, vertical = 8.dp), shadowElevation = 0.dp) {
                                Row(Modifier.widthIn(max = 360.dp).fillMaxWidth().padding(4.dp), horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                                    val labels = listOf(text("插件", "Plugins"), text("工作台", "Workspace"), text("设置", "Settings"))
                                    listOf(R.drawable.tab_plugins, R.drawable.tab_workspace, R.drawable.tab_settings).forEachIndexed { index, icon ->
                                        Column(horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.Center,
                                            modifier = Modifier.weight(1f).height(64.dp).clip(RoundedCornerShape(28.dp))
                                                .background(if (tab == index) MiuixTheme.colorScheme.primary.copy(alpha = 0.12f) else Color.Transparent)
                                                .selectable(selected = tab == index, role = Role.Tab, onClick = { if (tab == 1 && index == 1) app.chooseWorkspace() else app.showTab(index) }).testTag("native.tab.$index")) {
                                            Icon(ImageVector.vectorResource(icon), null, modifier = Modifier.size(26.dp), tint = if (tab == index) MiuixTheme.colorScheme.primary else MiuixTheme.colorScheme.onSurface)
                                            Text(labels[index], fontSize = 12.sp, color = if (tab == index) MiuixTheme.colorScheme.primary else MiuixTheme.colorScheme.onSurfaceVariantSummary)
                                        }
                                    }
                                }
                            }
                        } else {
                            NavigationBar(modifier = Modifier.testTag("native.tabs"), defaultWindowInsetsPadding = false) {
                                val labels = listOf(text("插件", "Plugins"), text("工作台", "Workspace"), text("设置", "Settings"))
                                val icons = listOf(R.drawable.tab_plugins, R.drawable.tab_workspace, R.drawable.tab_settings)
                                labels.forEachIndexed { index, label ->
                                    NavigationBarItem(
                                        selected = tab == index, onClick = { if (tab == 1 && index == 1) app.chooseWorkspace() else app.showTab(index) },
                                        icon = ImageVector.vectorResource(icons[index]), label = label,
                                        modifier = Modifier.testTag("native.tab.$index").then(if (tab == 1 && index == 1) Modifier
                                            .clickable { app.chooseWorkspace() }.pointerInput(Unit) { detectTapGestures { app.chooseWorkspace() } } else Modifier),
                                        colors = NavigationBarDefaults.navigationBarItemColors(selectedContentColor = MiuixTheme.colorScheme.primary),
                                    )
                                }
                            }
                        }
                    }
                },
            ) { padding ->
                Box(Modifier.fillMaxSize().padding(padding).testTag("native.body")) {
                    when (tab) { 0 -> Plugins(); 2 -> when (settingsPage) { "logs" -> Logs(); "runtimes" -> Runtimes(); "runtime-settings" -> RuntimeSettings(); else -> Settings() }; else -> Workspace() }
                }
                Dialogs()
            }
            if (drawer) WorkspaceDrawer()
            }
        }
    }

    @Composable private fun SettingsTopBar() {
        SmallTopAppBar(
            title = when (settingsPage) {
                "logs" -> text("日志", "Logs")
                "runtime-settings" -> text("Lua Host 设置", "Lua Host settings")
                else -> "Lua Host"
            },
            defaultWindowInsetsPadding = false,
            modifier = Modifier.testTag("native.settings-top-bar"),
            navigationIcon = {
                IconButton(onClick = { back() }, modifier = Modifier.testTag(if (settingsPage == "logs") "native.log-back" else "native.runtime-back")) {
                    Icon(ImageVector.vectorResource(R.drawable.action_back), text("返回", "Back"))
                }
            },
            actions = {
                when (settingsPage) {
                    "logs" -> {
                        IconButton(onClick = { app.showLogs() }, modifier = Modifier.testTag("native.log-refresh")) {
                            Icon(ImageVector.vectorResource(R.drawable.action_refresh), text("刷新", "Refresh"))
                        }
                        IconButton(onClick = { confirm(text("清空日志", "Clear logs"), text("删除已保存的安卓日志？", "Delete saved Android logs?")) { app.clearLogs() } }) {
                            Icon(ImageVector.vectorResource(R.drawable.action_remove), text("清空日志", "Clear logs"))
                        }
                    }
                    "runtimes" -> IconButton(onClick = { app.chooseRuntime() }, enabled = installingName.isEmpty()) {
                        Icon(ImageVector.vectorResource(R.drawable.action_install), text("导入运行时包", "Import runtime package"))
                    }
                    "runtime-settings" -> {
                        val config = runtimeSettings
                        if (config?.getJSONArray("fields")?.objects()?.any { !it.optBoolean("readonly") } == true)
                            Button(onClick = { app.saveRuntimeSettings(config.getString("hash"), JSONObject(runtimeValues.toMap())) }, enabled = !runtimeSettingsBusy) { Text(text("保存", "Save")) }
                    }
                }
            },
        )
    }

    @Composable private fun Workspace() {
        val current = web
        if (current != null) {
            key(current) {
                AndroidView(
                    modifier = Modifier.fillMaxSize(),
                    factory = { WorkspaceView(it) { app.chooseWorkspace() }.apply {
                        (current.parent as? ViewGroup)?.removeView(current)
                        addView(current, FrameLayout.LayoutParams(-1, -1))
                    } },
                    onRelease = { it.removeAllViews() },
                )
            }
        }
        if (!workspaceReady) Box(Modifier.fillMaxSize().background(MiuixTheme.colorScheme.surface), contentAlignment = Alignment.Center) {
            Text(text("正在连接工作台…", "Connecting to workspace…"), color = MiuixTheme.colorScheme.onSurface)
        } else if (current == null) Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
            Button(onClick = { app.showTab(1) }) { Text(text("重新打开工作台", "Reopen workspace")) }
        }
    }

    @Composable private fun Page(content: @Composable ColumnScope.() -> Unit) {
        Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp).padding(bottom = if (tab == 0) 144.dp else 24.dp), content = content)
    }
    @Composable private fun Section(title: String, content: @Composable ColumnScope.() -> Unit) {
        Text(title, modifier = Modifier.padding(start = 12.dp, top = 20.dp, bottom = 8.dp), fontSize = 14.sp, color = MiuixTheme.colorScheme.onSurfaceVariantSummary)
        Card(modifier = Modifier.fillMaxWidth(), content = content)
    }
    @Composable private fun Hint(value: String) {
        Text(value, modifier = Modifier.padding(horizontal = 16.dp, vertical = 10.dp), fontSize = 13.sp, color = MiuixTheme.colorScheme.onSurfaceVariantSummary)
    }
    @Composable private fun Entry(title: String, summary: String? = null, action: () -> Unit) {
        BasicComponent(title = title, summary = summary, onClick = action,
            endActions = { Text("›", color = MiuixTheme.colorScheme.onSurfaceVariantSummary, fontSize = 24.sp) })
    }

    @Composable private fun Choice(title: String, value: String, action: () -> Unit) {
        BasicComponent(title = title, onClick = action, endActions = { Text(value + "  ›", color = MiuixTheme.colorScheme.onSurfaceVariantSummary, fontSize = 14.sp) })
    }
    @Composable private fun Spinner(title: String, values: List<String>, selected: Int, tag: String, change: (Int) -> Unit) {
        OverlaySpinnerPreference(title = title, items = values.map { DropdownItem(text = it) }, selectedIndex = selected.coerceAtLeast(0),
            onSelectedIndexChange = change, modifier = Modifier.testTag(tag))
    }
    @Composable private fun Settings() = Page {
        Section(text("应用设置", "Application")) {
            SwitchPreference(title = text("仅远程模式", "Remote only"), checked = remoteOnly,
                summary = text("仅连接远程工作台", "Use remote workspaces only"), onCheckedChange = { app.setRemoteMode(it) }, modifier = Modifier.testTag("native.remote-only"))
            SwitchPreference(title = text("悬浮底栏", "Floating navigation"), checked = floatingTabs, onCheckedChange = { app.floatingTabs(it) }, modifier = Modifier.testTag("native.floating-tabs"))
        }
        Section(text("外观", "Appearance")) {
            Spinner(text("语言", "Language"), listOf("简体中文", "English"), if (locale == "en") 1 else 0, "native.language") { app.appearance(if (it == 1) "en" else "zh", theme) }
            Spinner(text("主题", "Theme"), listOf(text("浅色", "Light"), text("深色", "Dark")), if (theme == "dark") 1 else 0, "native.theme") { app.appearance(locale, if (it == 1) "dark" else "light") }
        }
        Section(text("通知", "Notifications")) {
            Spinner(text("通知方式", "Notification delivery"), listOf(text("系统通知", "System notifications"), text("工作台通知", "Workspace notifications")), if (systemNotifications) 0 else 1, "native.notifications") { app.notificationMode(it == 0) }
            if (systemNotifications && !notificationsAllowed) Entry(text("开启通知权限", "Allow notifications"), text("当前使用工作台通知", "Using workspace notifications for now")) { app.openSystemSettings("notifications") }
        }
        Section(text("网络", "Network")) {
            Entry(text("GitHub 下载代理", "GitHub download proxy"), githubProxy.ifEmpty { text("直连", "Direct connection") }) { dialog = "proxy" }
        }
        if (!remoteOnly) Section(text("后台运行", "Background operation")) {
            Entry(text("允许自启动", "Allow autostart"), text("重启后恢复计划任务", "Restore scheduled tasks after restart")) { app.openSystemSettings("autostart") }
            Entry(text("电池与省电策略", "Battery and power saving"), if (batteryExempt) text("已豁免系统电池优化", "Battery optimization exempt") else text("建议设为“无限制”", "Unrestricted is recommended")) { app.openSystemSettings("battery") }
        }
        if (!remoteOnly) Section(text("扩展与运行时", "Extensions and runtimes")) {
            Entry(text("运行时管理", "Runtimes"), text("安装和管理 Lua Host", "Install and manage Lua Host")) { app.showRuntimes() }
            Entry(text("功能扩展中心", "Functional extensions"), text("管理内置工作台的 .cphext 扩展", "Manage .cphext extensions for the built-in workspace")) { app.showExtensions() }
        }
        Section(text("日志", "Logs")) {
            SwitchPreference(title = text("记录安卓日志", "Record Android logs"), checked = logging, onCheckedChange = { app.logging(it, logLevel, logDays) }, modifier = Modifier.testTag("native.logging"))
            if (logging) {
                val levels = listOf("debug", "info", "warn", "error")
                val days = listOf(1, 3, 7, 14, 30)
                Spinner(text("日志级别", "Log level"), levels.map { it.uppercase() }, levels.indexOf(logLevel), "native.log-level") { app.logging(logging, levels[it], logDays) }
                Spinner(text("保留天数", "Retention"), days.map { "$it " + text("天", "days") }, days.indexOf(logDays), "native.log-days") { app.logging(logging, logLevel, days[it]) }
            }
            Entry(text("查看日志", "View logs")) { app.showLogs() }
        }
        Section(text("工作台界面", "Workspace interface")) {
            Entry(text("导入界面更新", "Import interface update"), text("导入已签名的 .cphui 界面包", "Import a signed .cphui interface package")) { app.chooseFrontend() }
            Entry(text("恢复内置界面", "Restore built-in interface")) {
                confirm(text("恢复内置界面", "Restore built-in interface"), text("已安装的界面更新将停用，业务数据会保留。", "Disable the interface update and keep business data.")) { app.resetFrontend() }
            }
        }
        Section(text("关于", "About")) {
            Row(Modifier.padding(16.dp), verticalAlignment = Alignment.CenterVertically) {
                Image(painterResource(R.drawable.cph_logo), "ClawProxyHub", modifier = Modifier.size(52.dp))
                Column(Modifier.padding(start = 16.dp)) {
                    Text("ClawProxyHub", fontSize = 20.sp, fontWeight = FontWeight.SemiBold)
                    Text(text("版本 ", "Version ") + BuildConfig.VERSION_NAME + " · Core " + BuildConfig.CORE_VERSION, fontSize = 14.sp, color = MiuixTheme.colorScheme.onSurfaceVariantSummary)
                }
            }
            Hint(text("自托管的账号管理与 API 代理网关", "Self-hosted account management and API gateway"))
            Entry(text("检查更新", "Check for updates")) { app.checkUpdate() }
            Entry(text("项目主页", "Project website")) { app.openURL("https://github.com/ShadowSmallBaby/ClawProxyHub") }
            Entry(text("开源许可", "Open-source licenses")) { dialog = "licenses" }
            Entry(text("系统应用信息", "System app information")) { app.openSystemSettings("details") }
        }
    }

    @Composable private fun Runtimes() = Page {
        if (readingRuntimes) Hint(text("正在读取运行时…", "Loading runtimes…"))
        runtimeError?.let { Hint(it) }
        val current = runtimeState
        Section("Lua Host") {
            if (current == null) Hint(text("尚未安装，可导入运行时包或从可用包中安装。", "Not installed. Import a runtime or select an available package."))
            else {
                val enabled = current.optBoolean("enabled")
                Hint(text("版本 ", "Version ") + current.getJSONObject("manifest").getString("version") + " · " + when {
                    !enabled -> text("已停用", "Disabled")
                    current.optBoolean("available") -> text("已启用", "Enabled")
                    else -> text("不可用", "Unavailable")
                })
                current.optString("error").takeIf { it.isNotEmpty() }?.let { Hint(it) }
                if ((current.getJSONObject("manifest").optJSONArray("settings")?.length() ?: 0) > 0)
                    Entry(text("设置", "Settings")) { app.showRuntimeSettings() }
                Entry(if (enabled) text("停用运行时", "Disable runtime") else text("启用运行时", "Enable runtime")) {
                    if (enabled) confirm(text("停用 Lua Host", "Disable Lua Host"), text("Lua 插件将停止运行，使用它的计划任务需先停用。", "Lua plugins will stop. Disable their scheduled tasks first.")) { app.runtimeChange("disable") }
                    else app.runtimeChange("enable")
                }
                Entry(text("卸载运行时", "Uninstall runtime")) {
                    confirm(text("卸载 Lua Host", "Uninstall Lua Host"), text("卸载后 Lua 插件无法运行，脚本和业务数据会保留。", "Lua plugins cannot run after uninstalling. Scripts and business data are kept.")) { app.runtimeChange("uninstall") }
                }
            }
        }
        Section(text("可用包", "Available packages")) {
            if (runtimePackages.isEmpty()) Hint(text("没有挂载的运行时包。", "No mounted runtime packages."))
            runtimePackages.forEach { item ->
                val manifest = item.optJSONObject("manifest")
                val status = item.optString("status")
                val label = (manifest?.optString("name") ?: "Lua Host") + " · " + (manifest?.optString("version") ?: "")
                if (manifest != null && status in listOf("available", "update", "removed", "blocked"))
                    Entry(label, text("点击安装", "Tap to install")) {
                        confirm(text("安装 Lua Host", "Install Lua Host"), label) { app.installMountedRuntime(item.getString("sha256"), manifest.getJSONArray("permissions")) }
                    }
                else Hint(label + " · " + when (status) {
                    "installed" -> text("已安装", "Installed")
                    "older" -> text("较旧版本", "Older version")
                    else -> text("不可用", "Unavailable")
                })
                item.optString("error").takeIf { it.isNotEmpty() }?.let { Hint(it) }
            }
        }
        Section(text("在线更新", "Online updates")) {
            Entry(text("检查运行时更新", "Check for runtime updates")) { app.checkRuntimeUpdate() }
            if (runtimeUpdateLoading) Hint(text("正在检查…", "Checking…"))
            runtimeUpdateError?.let { Hint(it) }
            latestRuntime?.let { item ->
                val version = item.getString("version")
                val currentVersion = current?.optJSONObject("manifest")?.optString("version")
                if (currentVersion == null || FrontendPackage.compare(version, currentVersion) > 0)
                    Entry("Lua Host · $version", text("下载并安装", "Download and install")) {
                        confirm(text("安装 Lua Host", "Install Lua Host"), "Lua Host · $version") { app.installRuntimeUpdate(item) }
                    }
                else Hint(text("已安装最新或更高版本", "The latest or a newer version is installed"))
            }
            if (runtimeUpdateChecked && !runtimeUpdateLoading && runtimeUpdateError == null && latestRuntime == null) Hint(text("当前发行没有兼容的运行时包。", "No compatible runtime in this release."))
        }
    }

    private fun settingText(value: JSONObject?, fallback: String = ""): String {
        if (value == null) return fallback
        return value.optString(locale).ifEmpty { value.optString("zh").ifEmpty { value.optString("en").ifEmpty { fallback } } }
    }

    // 运行时设置也从签名清单渲染；只读字段不提供伪开关。
    @Composable private fun RuntimeSettings() = Page {
        if (runtimeSettingsBusy) Hint(text("正在处理…", "Working…"))
        runtimeSettingsError?.let { Hint(it) }
        Section(text("Lua Host 设置", "Lua Host settings")) {
            runtimeSettings?.getJSONArray("fields")?.objects()?.forEach { field ->
                val id = field.getString("id")
                val label = settingText(field.optJSONObject("label"), id)
                val hint = settingText(field.optJSONObject("hint")).ifEmpty { null }
                val readonly = field.optBoolean("readonly")
                when (field.getString("kind")) {
                    "toggle" -> SwitchPreference(title = label, summary = hint, checked = runtimeValues[id] == true,
                        enabled = !readonly && !runtimeSettingsBusy, onCheckedChange = { runtimeValues[id] = it }, modifier = Modifier.testTag("native.runtime-setting.$id"))
                    "select" -> {
                        val options = field.getJSONArray("options").objects()
                        val selected = options.indexOfFirst { it.getString("value") == runtimeValues[id] }
                        if (readonly || runtimeSettingsBusy) BasicComponent(title = label, summary = options.getOrNull(selected)?.let { settingText(it.optJSONObject("label")) })
                        else Spinner(label, options.map { settingText(it.optJSONObject("label")) }, selected, "native.runtime-setting.$id") { runtimeValues[id] = options[it].getString("value") }
                        hint?.let { Hint(it) }
                    }
                    "text", "textarea" -> Column(Modifier.padding(16.dp)) {
                        TextField(value = runtimeValues[id] as? String ?: "", onValueChange = { value ->
                            if (value.codePointCount(0, value.length) <= field.optInt("max_length", 4096)) runtimeValues[id] = value
                        }, label = label, singleLine = field.getString("kind") == "text", enabled = !readonly && !runtimeSettingsBusy)
                        hint?.let { Hint(it) }
                    }
                }
            }
        }
    }

    @Composable private fun Logs() = Column(Modifier.fillMaxSize().padding(16.dp).testTag("native.log-page")) {
        Card(Modifier.weight(1f).fillMaxWidth()) {
            androidx.compose.foundation.text.selection.SelectionContainer {
                Text(logContent?.ifEmpty { text("暂无日志", "No logs yet") } ?: text("正在读取…", "Loading…"),
                    modifier = Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp).testTag("native.log-content"),
                    fontSize = 12.sp, fontFamily = androidx.compose.ui.text.font.FontFamily.Monospace)
            }
        }
    }

    @Composable private fun Plugins() = Column {
        TabRowWithContour(tabs = listOf(text("市场", "Market"), text("已安装", "Installed")), selectedTabIndex = pluginTab,
            onTabSelected = { pluginTab = it }, modifier = Modifier.fillMaxWidth().padding(vertical = 12.dp).testTag("native.plugin-tabs"))
        key(pluginTab) { if (pluginTab == 0) Market() else Installed() }
    }
    private fun operationText(): String = when (installPhase) {
        "waiting" -> text("准备安装…", "Preparing installation…")
        "connecting" -> text("正在连接…", "Connecting…")
        "downloading" -> if (totalBytes > 0) text("下载中 ", "Downloading ") + (receivedBytes * 100 / totalBytes).coerceIn(0, 100) + "%"
            else text("下载中 ", "Downloading ") + (receivedBytes / 1024) + " KiB"
        "verifying" -> text("正在校验…", "Verifying…")
        "installing" -> text("正在安装…", "Installing…")
        "starting" -> text("正在更新列表…", "Updating plugin list…")
        "complete" -> text("安装完成", "Installed")
        "failed" -> text("安装失败", "Installation failed")
        else -> ""
    }
    private fun label(item: JSONObject): String {
        val name = item.optString("name")
        return item.optJSONObject("label")?.optString(locale, name)?.takeIf { it.isNotBlank() } ?: name
    }
    @Composable private fun PluginIcon(item: JSONObject, local: Boolean) {
        val name = item.optString("name")
        val path = if (local) item.optString("local_icon") else ""
        val bitmap by produceState<ImageBitmap?>(null, name, item.optString("version"), path) {
            value = withContext(Dispatchers.IO) { runCatching {
                if (path.isEmpty()) null else {
                    val options = BitmapFactory.Options().apply { inJustDecodeBounds = true }
                    BitmapFactory.decodeFile(path, options)
                    if (options.outWidth <= 0 || options.outHeight <= 0 || options.outWidth > 8192 || options.outHeight > 8192) null else {
                        options.inSampleSize = (maxOf(options.outWidth, options.outHeight) / 128).coerceAtLeast(1)
                        options.inJustDecodeBounds = false
                        BitmapFactory.decodeFile(path, options)?.asImageBitmap()
                    }
                }
            }.getOrNull() }
        }
        Box(Modifier.size(52.dp).clip(RoundedCornerShape(14.dp)).background(if (bitmap != null) Color(0xfff2f2f2) else MiuixTheme.colorScheme.surfaceContainerHigh), contentAlignment = Alignment.Center) {
            if (bitmap != null) Image(bitmap!!, null, Modifier.fillMaxSize())
            else Text(label(item).take(1).uppercase(), fontSize = 24.sp, color = MiuixTheme.colorScheme.primary)
        }
    }
    @Composable private fun PluginCard(item: JSONObject, onClick: (() -> Unit)? = null, action: @Composable () -> Unit) {
        Card(Modifier.fillMaxWidth().padding(top = 12.dp).then(if (onClick != null) Modifier.clickable(onClick = onClick) else Modifier)) {
            Row(Modifier.fillMaxWidth().padding(16.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(14.dp)) {
                PluginIcon(item, onClick != null)
                Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(5.dp)) {
                    Text(label(item), fontSize = 18.sp, fontWeight = FontWeight.SemiBold, maxLines = 1, overflow = TextOverflow.Ellipsis)
                    Text(text("版本：", "Version: ") + item.optString("version"), fontSize = 13.sp, color = MiuixTheme.colorScheme.onSurfaceVariantSummary)
                    Text((if (item.optString("runtime") == "lua") "Lua" else "Go") + " · " + text("协议 ", "Protocol ") + item.optInt("protocol_version").let { if (it > 0) "v$it" else text("未声明", "Unspecified") }, fontSize = 12.sp, color = MiuixTheme.colorScheme.onSurfaceVariantSummary)
                }
                action()
            }
        }
    }
    private fun pluginStatusText(status: String) = when (status) {
        "runtime_loading" -> text("正在读取运行时", "Loading runtime")
        "runtime_missing" -> text("运行时未安装", "Runtime not installed")
        "runtime_stopped" -> text("运行时已停止", "Runtime stopped")
        "runtime_unavailable" -> text("运行时不可用", "Runtime unavailable")
        "unpublished" -> text("未发布安卓版", "Android package pending")
        "abi" -> text("不支持此架构", "Unsupported architecture")
        "not_installed" -> text("未安装", "Not installed")
        "update_available" -> text("可更新", "Update available")
        "installed" -> text("已安装", "Installed")
        "enabled" -> text("已启用", "Enabled")
        "disabled" -> text("已停用", "Disabled")
        "running" -> text("运行中", "Running")
        "stopped" -> text("已停止", "Stopped")
        "unavailable" -> text("不可用", "Unavailable")
        else -> text("下载信息不完整", "Incomplete download details")
    }
    @Composable private fun Market() = Column {
        var focused by remember { mutableStateOf(false) }
        val focus = LocalFocusManager.current
        InputField(query = search, onQueryChange = { search = it }, onSearch = { focus.clearFocus() }, expanded = focused,
            onExpandedChange = { focused = it }, label = text("搜索插件名称", "Search plugins by name"), modifier = Modifier.padding(horizontal = 16.dp).testTag("native.plugin-search"))
        Page {
            val filtered = marketItems.filter { search.isBlank() || it.optString("name").contains(search.trim(), true) || label(it).contains(search.trim(), true) }
            if (marketItems.isEmpty() && !marketLoading) Hint(text("市场暂不可用，可稍后刷新或从文件导入。", "Market unavailable. Refresh later or import from a file."))
            else if (filtered.isEmpty()) Hint(text("没有匹配的插件", "No matching plugins"))
            if (!marketOnline && marketItems.isNotEmpty()) Hint(text("当前显示已缓存的市场目录", "Showing the cached catalog"))
            filtered.forEach { entry ->
                val name = entry.optString("name")
                val local = installed.find { it.optString("name") == name }
                val status = PluginState.market(entry, local, luaRuntime)
                PluginCard(entry) {
                    if (installingName == name) Text(operationText(), fontSize = 12.sp, color = MiuixTheme.colorScheme.primary)
                    else Column(Modifier.widthIn(max = 136.dp), horizontalAlignment = Alignment.End) {
                        Text(pluginStatusText(status), fontSize = 12.sp, color = MiuixTheme.colorScheme.onSurfaceVariantSummary,
                            modifier = Modifier.testTag("native.plugin-status.$name").then(if (status.startsWith("runtime_") && status != "runtime_loading") Modifier.clickable { app.showRuntimes() } else Modifier))
                        if (status == "not_installed" || status == "update_available") {
                            IconButton(onClick = { app.installMarket(name, entry.optString("runtime") == "lua") }, enabled = installingName.isEmpty(), modifier = Modifier.size(44.dp)) {
                                Icon(ImageVector.vectorResource(if (local == null) R.drawable.action_install else R.drawable.action_refresh),
                                    if (local == null) text("安装", "Install") else text("更新", "Update"), tint = MiuixTheme.colorScheme.primary)
                            }
                        }
                    }
                }
            }
        }
    }
    @Composable private fun Installed() = Page {
        if (!remoteOnly) ExtensionContributions.list(pluginExtensions, "plugins.toolbar", false).objects().forEach { entry ->
            Card { Entry(contributionLabel(entry)) { app.openExtension(entry, "") } }
        }
        if (readingPlugins) Hint(text("正在读取插件…", "Loading plugins…"))
        else if (installed.isEmpty()) Hint(text("尚未安装插件，可以前往市场或从文件导入。", "No plugins installed. Browse the market or import a file."))
        installed.forEach { plugin ->
            val status = PluginState.installed(plugin, luaRuntime)
            PluginCard(plugin, onClick = { selectedPlugin = plugin; dialog = "plugin" }) {
                Text(pluginStatusText(status), modifier = Modifier.widthIn(max = 136.dp).testTag("native.plugin-status.${plugin.optString("name")}"),
                    fontSize = 12.sp, color = if (status == "running" || status == "enabled") MiuixTheme.colorScheme.primary else MiuixTheme.colorScheme.onSurfaceVariantSummary)
            }
        }
    }
    private fun contributionLabel(entry: JSONObject): String = entry.optJSONObject("labels")?.optString(locale)?.takeIf { it.isNotBlank() } ?: entry.optString("label")
    @Composable private fun WorkspaceDrawer() {
        var revealed by remember { mutableStateOf("") }
        Box(Modifier.fillMaxSize().testTag("native.workspace-drawer")) {
            Box(Modifier.fillMaxSize().background(Color.Black.copy(alpha = 0.32f)).clickable { drawer = false })
            Column(Modifier.fillMaxHeight().fillMaxWidth(0.88f).widthIn(max = 360.dp).background(MiuixTheme.colorScheme.surface).padding(16.dp)) {
                Text(text("工作台", "Workspaces"), modifier = Modifier.padding(12.dp), fontSize = 24.sp, fontWeight = FontWeight.SemiBold, color = MiuixTheme.colorScheme.onSurface)
                LazyColumn(Modifier.weight(1f).fillMaxWidth().testTag("native.workspace-list"), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    if (!remoteOnly) item("builtin") {
                        Card { Entry(text("内置工作台", "Built-in workspace"), if (currentConnection == WorkspaceStore.LOCAL) text("当前使用", "Selected") else null) { drawer = false; app.switchWorkspace(WorkspaceStore.LOCAL) } }
                    }
                    items(connections, key = { it.getString("id") }) { item ->
                        val id = item.getString("id")
                        var drag by remember { mutableFloatStateOf(0f) }
                        var dragging by remember { mutableStateOf(false) }
                        val limit = with(androidx.compose.ui.platform.LocalDensity.current) { 144.dp.toPx() }
                        Box(Modifier.fillMaxWidth().clip(RoundedCornerShape(16.dp)).testTag("native.workspace.$id")) {
                            Row(Modifier.matchParentSize().padding(end = 8.dp), horizontalArrangement = Arrangement.spacedBy(16.dp, Alignment.End), verticalAlignment = Alignment.CenterVertically) {
                                if (revealed == id || dragging) {
                                    IconButton(onClick = { revealed = ""; editingConnection = item; dialog = "add" }, backgroundColor = MiuixTheme.colorScheme.surfaceContainerHigh,
                                        modifier = Modifier.size(52.dp).testTag("native.workspace.edit.$id")) {
                                        Icon(ImageVector.vectorResource(R.drawable.action_edit), text("编辑工作台", "Edit workspace"), modifier = Modifier.size(24.dp), tint = MiuixTheme.colorScheme.onSurface)
                                    }
                                    IconButton(onClick = { revealed = ""; confirm(text("移除工作台", "Remove workspace"), text("移除这个工作台及其登录信息？", "Remove this workspace and its saved login?")) { app.removeWorkspace(id) } },
                                        backgroundColor = MiuixTheme.colorScheme.error.copy(alpha = 0.1f), modifier = Modifier.size(52.dp).testTag("native.workspace.remove.$id")) {
                                        Icon(ImageVector.vectorResource(R.drawable.action_remove), text("移除工作台", "Remove workspace"), modifier = Modifier.size(24.dp), tint = MiuixTheme.colorScheme.error)
                                    }
                                }
                            }
                            Card(Modifier.fillMaxWidth().offset { IntOffset((if (dragging) drag else if (revealed == id) -limit else 0f).roundToInt(), 0) }
                                .pointerInput(id, revealed) {
                                    detectHorizontalDragGestures(onDragStart = { dragging = true; drag = if (revealed == id) -limit else 0f },
                                        onHorizontalDrag = { change, amount -> change.consume(); drag = (drag + amount).coerceIn(-limit, 0f) },
                                        onDragEnd = { revealed = if (drag < -limit / 3) id else ""; dragging = false }, onDragCancel = { dragging = false })
                                }) {
                                Entry(item.getString("name"), if (id == currentConnection) text("当前使用", "Selected") else item.getString("baseURL")) {
                                    if (revealed == id) revealed = "" else { drawer = false; app.switchWorkspace(id) }
                                }
                            }
                        }
                    }
                }
                Spacer(Modifier.height(16.dp))
                Button(onClick = { editingConnection = null; dialog = "add" }, modifier = Modifier.fillMaxWidth().testTag("native.workspace.add")) { Text(text("添加工作台", "Add workspace")) }
            }
        }
    }
    @Composable private fun InstallProgress() {
        Text(operationName, fontWeight = FontWeight.SemiBold)
        LinearProgressIndicator(modifier = Modifier.fillMaxWidth().testTag("native.install-indicator"), progress = when {
            installPhase == "complete" -> 1f
            installingName.isEmpty() -> 0f
            installPhase == "downloading" && totalBytes > 0 -> (receivedBytes.toFloat() / totalBytes).coerceIn(0f, 1f)
            else -> null
        })
        Text(operationText(), color = if (installPhase == "failed") MiuixTheme.colorScheme.error else MiuixTheme.colorScheme.primary)
        val scroll = rememberScrollState()
        LaunchedEffect(installLog.size) { scroll.scrollTo(scroll.maxValue) }
        Column(Modifier.fillMaxWidth().height(200.dp).clip(RoundedCornerShape(12.dp)).background(MiuixTheme.colorScheme.surfaceContainerHigh)
            .verticalScroll(scroll).padding(12.dp).testTag("native.install-log"), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            installLog.forEach { Text(it, fontSize = 12.sp, color = MiuixTheme.colorScheme.onSurfaceVariantSummary) }
        }
    }

    @Composable private fun Dialogs() {
        val kind = dialog
        val title = when (kind) {
            "progress" -> text("安装插件", "Install plugin")
            "proxy" -> text("GitHub 下载代理", "GitHub download proxy")
            "update" -> text("检查更新", "Check for updates")
            "add" -> if (editingConnection == null) text("添加远程工作台", "Add remote workspace") else text("编辑工作台", "Edit workspace")
            "import" -> text("导入插件", "Import plugin")
            "lua" -> text("导入 Lua 脚本", "Import Lua script")
            "plugin" -> selectedPlugin?.let { label(it) } ?: ""
            "licenses" -> text("开源许可", "Open-source licenses")
            else -> dialogTitle
        }
        WindowDialog(show = kind.isNotEmpty(), title = title, onDismissRequest = { dismissDialog() }) {
            Column(Modifier.heightIn(max = 520.dp).verticalScroll(rememberScrollState()).semantics { testTagsAsResourceId = true }.testTag("native.dialog.$kind"), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                when (kind) {
                    "progress" -> InstallProgress()
                    "proxy" -> {
                        var address by rememberSaveable { mutableStateOf(githubProxy) }
                        var error by remember { mutableStateOf<String?>(null) }
                        TextField(value = address, onValueChange = { address = it; error = null }, label = "https://", singleLine = true)
                        Hint(text("用于市场、插件和应用更新，留空直连", "Used for the market, plugins and app updates. Leave empty for direct access"))
                        error?.let { Text(it, color = MiuixTheme.colorScheme.error) }
                        Button(onClick = { error = app.githubProxy(address); if (error == null) dialog = "" }, modifier = Modifier.fillMaxWidth()) { Text(text("保存", "Save")) }
                    }
                    "update" -> {
                        Text(text("当前版本 ", "Current version ") + BuildConfig.VERSION_NAME + " · Core " + BuildConfig.CORE_VERSION)
                        when {
                            updateError != null -> Text(updateError!!)
                            update == null -> Column(Modifier.fillMaxWidth().padding(vertical = 16.dp).testTag("native.update-loading"),
                                horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(12.dp)) {
                                CircularProgressIndicator(modifier = Modifier.testTag("native.update-spinner"), size = 24.dp)
                                Text(text("正在检查…", "Checking…"))
                            }
                            !update!!.optBoolean("newer") -> Text(text("已是最新版本", "You're up to date"))
                            update!!.optString("url").isEmpty() -> Text(text("最新发布暂无适用的安卓安装包", "The latest release has no APK for this device"))
                            else -> {
                                Text(text("发现新版本 ", "New release ") + update!!.optString("version"))
                                val items = update!!.optJSONObject("changelog")?.optJSONArray("items")
                                if (items != null) for (i in 0 until items.length()) {
                                    val item = items.optJSONObject(i) ?: continue
                                    Text(text(item.optString("zh", item.optString("en")), item.optString("en", item.optString("zh"))))
                                }
                                Button(onClick = { app.downloadUpdate(update!!.getString("url")) }, modifier = Modifier.fillMaxWidth()) { Text(text("下载更新", "Download update")) }
                            }
                        }
                    }
                    "import" -> {
                        Entry(text("插件包", "Plugin package"), ".cphplugin") { dialog = ""; app.choosePlugin() }
                        Entry(text("Lua 脚本", "Lua script"), ".lua") { dialog = ""; app.chooseLua() }
                    }
                    "plugin" -> selectedPlugin?.let { plugin ->
                        val name = plugin.getString("name")
                        val lua = plugin.optString("runtime") == "lua"
                        val enabled = plugin.optBoolean("enabled")
                        val dependency = PluginState.dependency(plugin, luaRuntime)
                        if (!remoteOnly) ExtensionContributions.list(pluginExtensions, "plugins.item.actions", plugin.optBoolean("editable")).objects().forEach { entry ->
                            Entry(contributionLabel(entry)) { dialog = ""; app.openExtension(entry, name) }
                        }
                        if (!plugin.optBoolean("available", true)) Hint(text("请重新导入可信版本。", "Import a trusted version again."))
                        if (dependency.isNotEmpty()) Entry("Lua Host", pluginStatusText(dependency)) { dialog = ""; app.showRuntimes() }
                        if (enabled || dependency.isEmpty()) Entry(if (enabled) text("停用", "Disable") else text("启用", "Enable")) { dialog = ""; app.togglePlugin(name, lua, !enabled) }
                        if (!lua && plugin.optBoolean("rollback")) Entry(text("恢复上一版本", "Restore previous version")) { dialog = ""; app.rollbackPlugin(name) }
                        Entry(text("移除插件", "Remove plugin")) {
                            confirm(text("移除插件", "Remove plugin"), text("移除 ", "Remove ") + label(plugin) + text("？使用它的任务将停止。", "? Work using this plugin will stop.")) { app.removePlugin(name, lua) }
                        }
                    }
                    "licenses" -> {
                        Entry("Miuix", "Apache License 2.0") { app.openURL("https://github.com/compose-miuix-ui/miuix/blob/main/LICENSE") }
                        Entry("AndroidX / Jetpack Compose", "Apache License 2.0") { app.openURL("https://android.googlesource.com/platform/frameworks/support/+/androidx-main/LICENSE.txt") }
                        Entry("Kotlin", "Apache License 2.0") { app.openURL("https://github.com/JetBrains/kotlin/blob/master/license/LICENSE.txt") }
                    }
                    "add" -> {
                        var name by rememberSaveable(editingConnection) { mutableStateOf(editingConnection?.optString("name") ?: "") }
                        var address by rememberSaveable(editingConnection) { mutableStateOf(editingConnection?.optString("baseURL") ?: "") }
                        var error by remember { mutableStateOf<String?>(null) }
                        TextField(value = name, onValueChange = { name = it }, label = text("名称", "Name"), singleLine = true)
                        if (editingConnection == null) TextField(value = address, onValueChange = { address = it; error = null }, label = "https://cph.example.com", singleLine = true)
                        error?.let { Text(it, color = MiuixTheme.colorScheme.error) }
                        Button(onClick = { error = app.saveWorkspace(editingConnection?.optString("id") ?: "", name, address); if (error == null) { dialog = ""; if (editingConnection == null) drawer = false } }, modifier = Modifier.fillMaxWidth()) { Text(text("保存", "Save")) }
                    }
                    "lua" -> {
                        var name by rememberSaveable { mutableStateOf("") }
                        TextField(value = name, onValueChange = { name = it }, label = text("脚本名称", "Script name"), singleLine = true)
                        Button(onClick = { dialog = ""; luaSource?.let { app.importLua(it, name) } }, enabled = name.matches(Regex("[a-z][a-z0-9_-]{0,63}")), modifier = Modifier.fillMaxWidth()) { Text(text("导入", "Import")) }
                    }
                    "confirm", "error" -> {
                        Text(dialogMessage)
                        Button(onClick = { dialog = ""; if (kind == "confirm") confirmation() }, modifier = Modifier.fillMaxWidth()) { Text(text("确定", "Confirm")) }
                    }
                }
                Button(onClick = { dismissDialog() }, modifier = Modifier.fillMaxWidth().testTag("native.dialog.close")) { Text(if (kind == "progress" && installingName.isNotEmpty()) text("收起", "Minimize") else if (kind in listOf("error", "progress", "update")) text("关闭", "Close") else text("取消", "Cancel")) }
            }
        }
    }
}

private fun JSONArray.objects() = (0 until length()).mapNotNull { optJSONObject(it) }
