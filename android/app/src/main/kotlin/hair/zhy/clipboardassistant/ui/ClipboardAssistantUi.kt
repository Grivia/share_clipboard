package hair.zhy.clipboardassistant.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ExitToApp
import androidx.compose.material.icons.automirrored.filled.Send
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Phone
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material.icons.filled.Share
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import hair.zhy.clipboardassistant.data.RepositoryState
import hair.zhy.clipboardassistant.data.model.DeviceModel

@Composable
fun ClipboardAssistantRoot(state: RepositoryState, model: AppViewModel) {
    when {
        !state.initialized -> LoadingScreen()
        !state.authenticated -> LoginScreen(state, model)
        else -> MainScreen(state, model)
    }
}

@Composable
private fun LoadingScreen() {
    Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        CircularProgressIndicator()
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun LoginScreen(state: RepositoryState, model: AppViewModel) {
    var server by remember(state.serverUrl) { mutableStateOf(state.serverUrl) }
    var account by remember(state.account) { mutableStateOf(state.account) }
    var password by remember { mutableStateOf("") }
    Scaffold(topBar = { TopAppBar(title = { Text("粘贴板助手") }) }) { padding ->
        Box(
            Modifier.fillMaxSize().padding(padding).padding(horizontal = 24.dp),
            contentAlignment = Alignment.Center,
        ) {
            Column(
                Modifier.fillMaxWidth().widthIn(max = 560.dp).verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(16.dp),
            ) {
                Icon(
                    Icons.Default.Share,
                    contentDescription = null,
                    tint = MaterialTheme.colorScheme.primary,
                    modifier = Modifier.size(36.dp),
                )
                Text("登录或注册", style = MaterialTheme.typography.headlineSmall)
                OutlinedTextField(
                    value = server,
                    onValueChange = { server = it },
                    label = { Text("服务端") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
                OutlinedTextField(
                    value = account,
                    onValueChange = { account = it },
                    label = { Text("账号") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
                OutlinedTextField(
                    value = password,
                    onValueChange = { password = it },
                    label = { Text("密码") },
                    visualTransformation = PasswordVisualTransformation(),
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
                state.error?.let { Text(it, color = MaterialTheme.colorScheme.error) }
                Button(
                    onClick = { model.authenticate(server, account, password) },
                    enabled = !state.busy && account.trim().isNotEmpty() && password.length >= 4,
                    modifier = Modifier.fillMaxWidth().height(48.dp),
                ) {
                    if (state.busy) CircularProgressIndicator(Modifier.size(20.dp), strokeWidth = 2.dp)
                    else Text("登录或注册")
                }
            }
        }
    }
}

private data class Destination(val label: String, val icon: ImageVector)

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun MainScreen(state: RepositoryState, model: AppViewModel) {
    val destinations = listOf(
        Destination("同步", Icons.Default.Refresh),
        Destination("设备", Icons.Default.Phone),
        Destination("设置", Icons.Default.Settings),
    )
    var selected by remember { mutableIntStateOf(0) }
    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(destinations[selected].label) },
                actions = {
                    IconButton(onClick = model::refresh, enabled = !state.busy) {
                        Icon(Icons.Default.Refresh, contentDescription = "立即刷新")
                    }
                },
            )
        },
        bottomBar = {
            NavigationBar {
                destinations.forEachIndexed { index, destination ->
                    NavigationBarItem(
                        selected = selected == index,
                        onClick = {
                            selected = index
                            if (index == 1) model.refreshDevices()
                        },
                        icon = { Icon(destination.icon, contentDescription = destination.label) },
                        label = { Text(destination.label) },
                    )
                }
            }
        },
    ) { padding ->
        Box(Modifier.fillMaxSize().padding(padding), contentAlignment = Alignment.TopCenter) {
            when (selected) {
                0 -> SyncScreen(state, model)
                1 -> DevicesScreen(state.devices, model)
                else -> SettingsScreen(state, model)
            }
        }
    }
}

@Composable
private fun SyncScreen(state: RepositoryState, model: AppViewModel) {
    Column(
        Modifier.fillMaxWidth().widthIn(max = 720.dp).padding(20.dp).verticalScroll(rememberScrollState()),
        verticalArrangement = Arrangement.spacedBy(18.dp),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            Icon(
                if (state.connected) Icons.Default.CheckCircle else Icons.Default.Refresh,
                contentDescription = null,
                tint = if (state.connected) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.secondary,
            )
            Column {
                Text(state.status, style = MaterialTheme.typography.titleMedium)
                Text(state.account, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
        state.error?.let { Text(it, color = MaterialTheme.colorScheme.error) }
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            Button(onClick = model::sendClipboard, enabled = state.syncEnabled, modifier = Modifier.weight(1f)) {
                Icon(Icons.AutoMirrored.Filled.Send, contentDescription = null)
                Spacer(Modifier.size(8.dp))
                Text("发送剪贴板")
            }
            OutlinedButton(
                onClick = model::copyLatest,
                enabled = state.latestText != null,
                modifier = Modifier.weight(1f),
            ) {
                Icon(Icons.Default.Share, contentDescription = null)
                Spacer(Modifier.size(8.dp))
                Text("复制最新")
            }
        }
        HorizontalDivider()
        Text("最新内容", style = MaterialTheme.typography.titleMedium)
        Surface(
            color = MaterialTheme.colorScheme.surfaceContainer,
            shape = MaterialTheme.shapes.small,
            modifier = Modifier.fillMaxWidth(),
        ) {
            SelectionContainer {
                Text(
                    state.latestText ?: "暂无远端内容",
                    modifier = Modifier.padding(16.dp),
                    style = MaterialTheme.typography.bodyLarge,
                    maxLines = 10,
                    overflow = TextOverflow.Ellipsis,
                )
            }
        }
        state.latestOrigin?.let { Text("来自 $it", color = MaterialTheme.colorScheme.onSurfaceVariant) }
        if (state.pendingCount > 0) {
            Text("${state.pendingCount} 条内容等待发送", color = MaterialTheme.colorScheme.tertiary)
        }
    }
}

@Composable
private fun DevicesScreen(devices: List<DeviceModel>, model: AppViewModel) {
    LazyColumn(
        modifier = Modifier.fillMaxWidth().widthIn(max = 720.dp),
        contentPadding = PaddingValues(vertical = 8.dp),
    ) {
        items(devices, key = DeviceModel::id) { device ->
            DeviceRow(device, model)
            HorizontalDivider(Modifier.padding(horizontal = 16.dp))
        }
    }
}

@Composable
private fun DeviceRow(device: DeviceModel, model: AppViewModel) {
    ListItem(
        headlineContent = {
            Text(if (device.current) "${device.displayName} · 本机" else device.displayName)
        },
        supportingContent = {
            Text(
                when {
                    device.revokedAt != null -> "已移除"
                    device.online -> "在线"
                    device.loggedIn -> "已登录"
                    else -> "已退出"
                },
            )
        },
        leadingContent = { Icon(Icons.Default.Phone, contentDescription = null) },
        trailingContent = {
            if (!device.current && device.revokedAt == null) {
                IconButton(onClick = { model.revokeDevice(device) }) {
                    Icon(Icons.Default.Delete, contentDescription = "移除此设备")
                }
            }
        },
    )
}

@Composable
private fun SettingsScreen(state: RepositoryState, model: AppViewModel) {
    Column(
        Modifier.fillMaxWidth().widthIn(max = 720.dp).padding(vertical = 8.dp),
    ) {
        ListItem(
            headlineContent = { Text("同步剪贴板") },
            trailingContent = {
                Switch(checked = state.syncEnabled, onCheckedChange = model::setSyncEnabled)
            },
            modifier = Modifier.selectable(selected = state.syncEnabled, onClick = {
                model.setSyncEnabled(!state.syncEnabled)
            }),
        )
        HorizontalDivider(Modifier.padding(horizontal = 16.dp))
        ListItem(headlineContent = { Text("账号") }, supportingContent = { Text(state.account) })
        ListItem(headlineContent = { Text("服务端") }, supportingContent = { Text(state.serverUrl) })
        HorizontalDivider(Modifier.padding(horizontal = 16.dp))
        ListItem(
            headlineContent = { Text("退出登录", color = MaterialTheme.colorScheme.error) },
            leadingContent = {
                Icon(
                    Icons.AutoMirrored.Filled.ExitToApp,
                    contentDescription = null,
                    tint = MaterialTheme.colorScheme.error,
                )
            },
            modifier = Modifier.selectable(selected = false, onClick = model::logout),
        )
    }
}
