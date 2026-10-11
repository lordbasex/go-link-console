// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.player.ui

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.widget.Toast
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.combinedClickable
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.sp
import org.golink.player.BuildConfig
import org.golink.player.R

/** "go-link Console Player v0.1.4 (build 104)", plus " · debug" in debug builds. */
@Composable
fun appVersionText(): String =
    stringResource(R.string.app_version, BuildConfig.VERSION_NAME, BuildConfig.VERSION_CODE) +
        if (BuildConfig.DEBUG) stringResource(R.string.app_version_debug) else ""

/** The app's version, small and faint; a long press copies it (for bug reports). */
@OptIn(ExperimentalFoundationApi::class)
@Composable
fun AppVersion(modifier: Modifier = Modifier) {
    val context = LocalContext.current
    val text = appVersionText()
    val copied = stringResource(R.string.app_version_copied)
    Text(
        text,
        color = Tokens.faint,
        fontSize = 12.sp,
        textAlign = TextAlign.Center,
        modifier = modifier
            .testTag("app-version")
            .combinedClickable(onClick = {}, onLongClick = {
                val cm = context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
                cm.setPrimaryClip(ClipData.newPlainText("go-link Console Player", text))
                Toast.makeText(context, copied, Toast.LENGTH_SHORT).show()
            }),
    )
}
