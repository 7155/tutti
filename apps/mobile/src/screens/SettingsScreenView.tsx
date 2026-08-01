import {
  NativeAvatar,
  NativeButton,
  NativeIconButton,
  NativeListRow,
  type NativeTheme,
  useNativeTheme
} from "@tutti-os/ui-system/native";
import { ScrollView, StyleSheet, Text, View } from "react-native";
import { t } from "../i18n";
import type { AccountSession } from "../services/mobileDomain";

export interface SettingsScreenViewProps {
  appVersion: string;
  onBack(): void;
  onSignOut(): void;
  session: AccountSession;
}

export function SettingsScreenView({
  appVersion,
  onBack,
  onSignOut,
  session
}: SettingsScreenViewProps) {
  const theme = useNativeTheme();
  const styles = createStyles(theme);
  const accountLabel = session.name || session.email || t("appName");

  return (
    <View style={styles.root}>
      <View style={styles.header}>
        <NativeIconButton
          accessibilityLabel={t("back")}
          icon={<Text style={styles.backIcon}>‹</Text>}
          onPress={onBack}
        />
        <Text style={styles.title}>{t("settings")}</Text>
        <View style={styles.headerSpacer} />
      </View>

      <ScrollView contentContainerStyle={styles.content}>
        <View style={styles.accountCard}>
          <NativeAvatar
            label={accountLabel}
            size="large"
            src={session.avatarURL}
          />
          <View style={styles.accountCopy}>
            <Text numberOfLines={1} style={styles.accountName}>
              {accountLabel}
            </Text>
            {session.email ? (
              <Text numberOfLines={1} style={styles.accountEmail}>
                {session.email}
              </Text>
            ) : null}
          </View>
        </View>

        <View style={styles.section}>
          <Text style={styles.sectionTitle}>{t("app")}</Text>
          <View style={styles.listCard}>
            <NativeListRow
              description={t("versionLabel", { version: appVersion })}
              title={t("softwareUpdate")}
            />
            <View style={styles.separator} />
            <NativeListRow
              description={t("aboutTuttiDescription")}
              title={t("aboutTutti")}
            />
          </View>
        </View>

        <NativeButton
          label={t("logout")}
          onPress={onSignOut}
          size="large"
          variant="destructiveGhost"
        />
      </ScrollView>
    </View>
  );
}

function createStyles(theme: NativeTheme) {
  return StyleSheet.create({
    accountCard: {
      alignItems: "center",
      backgroundColor: theme.color.panel,
      borderColor: theme.color.border,
      borderRadius: theme.radius.large,
      borderWidth: StyleSheet.hairlineWidth,
      flexDirection: "row",
      padding: theme.space.medium
    },
    accountCopy: { flex: 1, marginLeft: theme.space.medium },
    accountEmail: {
      color: theme.color.muted,
      fontSize: theme.space.small + 3,
      marginTop: theme.space.small / 2
    },
    accountName: {
      color: theme.color.text,
      fontSize: theme.space.medium + 2,
      fontWeight: "700"
    },
    backIcon: {
      color: theme.color.text,
      fontSize: theme.space.xlarge,
      lineHeight: theme.control.icon
    },
    content: {
      gap: theme.space.xlarge,
      padding: theme.space.large
    },
    header: {
      alignItems: "center",
      borderBottomColor: theme.color.border,
      borderBottomWidth: StyleSheet.hairlineWidth,
      flexDirection: "row",
      paddingHorizontal: theme.space.small,
      paddingVertical: theme.space.small / 2
    },
    headerSpacer: { height: theme.control.icon, width: theme.control.icon },
    listCard: {
      backgroundColor: theme.color.panel,
      borderColor: theme.color.border,
      borderRadius: theme.radius.large,
      borderWidth: StyleSheet.hairlineWidth,
      overflow: "hidden"
    },
    root: { backgroundColor: theme.color.background, flex: 1 },
    section: { gap: theme.space.small },
    sectionTitle: {
      color: theme.color.muted,
      fontSize: theme.space.small + 2,
      fontWeight: "700",
      paddingHorizontal: theme.space.small
    },
    separator: {
      backgroundColor: theme.color.border,
      height: StyleSheet.hairlineWidth,
      marginLeft: theme.space.medium
    },
    title: {
      color: theme.color.text,
      flex: 1,
      fontSize: theme.space.medium + 2,
      fontWeight: "700",
      textAlign: "center"
    }
  });
}
