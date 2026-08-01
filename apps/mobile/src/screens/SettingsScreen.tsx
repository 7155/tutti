import type { NativeStackScreenProps } from "@react-navigation/native-stack";
import { Alert } from "react-native";
import { useServiceSnapshot } from "../bindings/useServiceSnapshot";
import { t } from "../i18n";
import { mobileSecurity } from "../native/mobileNative";
import type { MobileRootStackParamList } from "../navigation/mobileNavigation";
import type { MobileApplicationService } from "../services/mobileApplicationService";
import { SettingsScreenView } from "./SettingsScreenView";

type Props = NativeStackScreenProps<MobileRootStackParamList, "Settings"> & {
  application: MobileApplicationService;
};

export function SettingsScreen({ application, navigation }: Props) {
  const snapshot = useServiceSnapshot(application);
  if (snapshot.status !== "authenticated") return null;

  const confirmSignOut = () => {
    Alert.alert(t("signOutConfirmTitle"), t("signOutConfirmDescription"), [
      { style: "cancel", text: t("cancel") },
      {
        onPress: () => void application.signOut(),
        style: "destructive",
        text: t("logout")
      }
    ]);
  };

  return (
    <SettingsScreenView
      appVersion={mobileSecurity.clientVersion}
      onBack={() => navigation.goBack()}
      onSignOut={confirmSignOut}
      session={snapshot.session}
    />
  );
}
