// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.player.ui

import android.content.Intent
import android.net.Uri
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Headphones
import androidx.compose.material.icons.filled.Link
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material3.Checkbox
import androidx.compose.material3.CheckboxDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.TextRange
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.TextFieldValue
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import org.golink.player.Prefs
import org.golink.player.R
import org.golink.player.core.CodeInput
import org.golink.player.core.InviteTarget
import org.golink.player.core.Invites
import org.golink.player.core.PlayerName
import org.golink.player.core.Terms

/**
 * The PIN form, like the website's JoinForm: the invitation (fixed when it
 * came from a QR code or a link, or typed: the 9-digit code or the link),
 * the 6-digit PIN the host gave this person, and the terms. The PIN is
 * always asked on a new join, and never travels in a link or a QR code.
 *
 * A group invitation (a link with #k=<key>, the website's KnockPrompt)
 * asks no PIN: a name the host recognises, and the terms the first time.
 * onJoin gets the target, the PIN ("" for a group invitation) and the name.
 */
@Composable
fun JoinScreen(
    fixedTarget: InviteTarget?,
    termsAccepted: Boolean,
    savedName: String,
    onJoin: (InviteTarget, String, String) -> Unit,
    onBack: () -> Unit,
) {
    val context = LocalContext.current
    // Shown grouped as "915 355 636" while typed (CodeInput); a link stays as typed.
    var codeField by rememberSaveable(stateSaver = TextFieldValue.Saver) { mutableStateOf(TextFieldValue("")) }
    val code = codeField.text
    var pin by rememberSaveable { mutableStateOf("") }
    var agreed by rememberSaveable { mutableStateOf(termsAccepted) }
    var askTerms by remember { mutableStateOf(false) }
    var name by rememberSaveable { mutableStateOf(savedName) }
    val target = fixedTarget ?: Invites.parseTyped(code)
    val group = (target as? InviteTarget.Link)?.group == true
    val ready = target != null && if (group) PlayerName.isValid(name) else Invites.isPin(pin)
    val submit = {
        if (target != null && ready) {
            if (!agreed) askTerms = true else onJoin(target, if (group) "" else pin, if (group) PlayerName.normalize(name) else "")
        }
    }
    val fieldColors = OutlinedTextFieldDefaults.colors(
        focusedBorderColor = Tokens.accent,
        unfocusedBorderColor = Tokens.border,
        focusedContainerColor = Tokens.sunken,
        unfocusedContainerColor = Tokens.sunken,
        cursorColor = Tokens.accent,
    )
    Column(Modifier.fillMaxSize().safeDrawingPadding().imePadding()) {
        Row(Modifier.fillMaxWidth().padding(8.dp), verticalAlignment = Alignment.CenterVertically) {
            IconButton(onClick = onBack, modifier = Modifier.size(48.dp)) {
                Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = stringResource(R.string.back), tint = Tokens.text)
            }
            Text(stringResource(if (group) R.string.knock_title else R.string.join_title), color = Tokens.text, fontSize = 20.sp, fontWeight = FontWeight.SemiBold)
        }
        Box(Modifier.fillMaxSize().verticalScroll(rememberScrollState()), contentAlignment = Alignment.TopCenter) {
            Column(Modifier.widthIn(max = 480.dp).fillMaxWidth().padding(20.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
                Card {
                    if (fixedTarget != null) {
                        Text(stringResource(R.string.join_invitation), color = Tokens.muted, fontSize = 13.sp)
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Icon(Icons.Filled.Link, contentDescription = null, tint = Tokens.accent, modifier = Modifier.size(20.dp))
                            Text(
                                fixedTarget.display(),
                                color = Tokens.text,
                                fontFamily = FontFamily.Monospace,
                                fontSize = 14.sp,
                                modifier = Modifier.padding(start = 8.dp),
                            )
                        }
                    } else {
                        OutlinedTextField(
                            value = codeField,
                            onValueChange = { v ->
                                codeField = if (v.text == codeField.text) {
                                    v // only the caret or the selection moved
                                } else {
                                    val edit = CodeInput.fromChange(codeField.text, v.text, v.selection.end)
                                    TextFieldValue(edit.text, TextRange(edit.caret))
                                }
                            },
                            label = { Text(stringResource(R.string.join_code_label)) },
                            placeholder = { Text("123 456 789") },
                            singleLine = true,
                            textStyle = TextStyle(fontFamily = FontFamily.Monospace, fontSize = 18.sp),
                            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri, autoCorrectEnabled = false, imeAction = ImeAction.Next),
                            isError = code.isNotBlank() && target == null,
                            supportingText = { Text(stringResource(R.string.join_code_hint)) },
                            colors = fieldColors,
                            modifier = Modifier.fillMaxWidth().testTag("join-code"),
                        )
                    }
                    if (group) {
                        // A group invitation: the host lets each person in by name.
                        Text(stringResource(R.string.knock_text), color = Tokens.text2, fontSize = 14.sp)
                        NameField(name, { name = it }, tag = "knock-name", hintTag = "knock-name-hint", label = stringResource(R.string.knock_name), onDone = { submit() })
                    } else OutlinedTextField(
                        value = pin,
                        onValueChange = { v -> pin = v.filter { it.isDigit() }.take(6) },
                        label = { Text(stringResource(R.string.join_pin_label)) },
                        placeholder = { Text("000000") },
                        leadingIcon = { Icon(Icons.Filled.Lock, contentDescription = null) },
                        singleLine = true,
                        textStyle = TextStyle(fontFamily = FontFamily.Monospace, fontSize = 22.sp, letterSpacing = 6.sp),
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.NumberPassword, imeAction = ImeAction.Go),
                        keyboardActions = KeyboardActions(onGo = { submit() }),
                        supportingText = { Text(stringResource(R.string.join_pin_hint)) },
                        colors = fieldColors,
                        modifier = Modifier.fillMaxWidth().testTag("join-pin"),
                    )
                    TermsCheck(agreed, { agreed = it; askTerms = false }, askTerms) { url ->
                        context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url)))
                    }
                    PrimaryButton(
                        stringResource(if (group) R.string.knock_send else R.string.join_button),
                        { submit() },
                        Modifier.fillMaxWidth().testTag("join-button"),
                        enabled = ready,
                    )
                }
            }
        }
    }
}

/** The terms checkbox, with links to the terms of use and the privacy policy. */
@Composable
fun TermsCheck(checked: Boolean, onChange: (Boolean) -> Unit, showError: Boolean, openUrl: (String) -> Unit) {
    Column {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Checkbox(
                checked = checked,
                onCheckedChange = onChange,
                colors = CheckboxDefaults.colors(checkedColor = Tokens.accent, checkmarkColor = Tokens.onAccent),
                modifier = Modifier.testTag("terms-check"),
            )
            Text(stringResource(R.string.join_terms), color = Tokens.text2, fontSize = 14.sp)
        }
        Row {
            TextButton(onClick = { openUrl(Terms.TERMS_URL) }) { Text(stringResource(R.string.join_terms_link), color = Tokens.accent) }
            TextButton(onClick = { openUrl(Terms.PRIVACY_URL) }) { Text(stringResource(R.string.join_privacy_link), color = Tokens.accent) }
        }
        if (showError) Notice(stringResource(R.string.join_terms_required), danger = true)
    }
}

/**
 * Before the first room on Android 12+: the Bluetooth permission, so a
 * headset carries the sound and the voice. Without it the loudspeaker (or
 * a wired headset) is used.
 */
@Composable
fun HeadphonesScreen(prefs: Prefs, onDone: () -> Unit) {
    val permission = Perms.BLUETOOTH
    val launcher = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) {
        if (permission != null) prefs.markAsked(permission)
        onDone()
    }
    Box(Modifier.fillMaxSize().safeDrawingPadding(), contentAlignment = Alignment.Center) {
        PermissionExplainer(
            icon = Icons.Filled.Headphones,
            title = stringResource(R.string.perm_bt_title),
            text = stringResource(R.string.perm_bt_text),
            allow = stringResource(R.string.perm_allow),
            notNow = stringResource(R.string.perm_not_now),
            onAllow = { if (permission != null) launcher.launch(permission) else onDone() },
            onNotNow = {
                if (permission != null) prefs.markAsked(permission)
                onDone()
            },
        )
    }
}
