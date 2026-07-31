import { act, create, type ReactTestRenderer } from "react-test-renderer";
import { KeyboardAvoidingView, Platform, Text } from "react-native";
import {
  MobileKeyboardAvoidingView,
  mobileKeyboardDismissMode
} from "./MobileKeyboardAvoidingView";

jest.mock("react-native-safe-area-context", () => ({
  useSafeAreaInsets: () => ({ bottom: 12, left: 0, right: 0, top: 24 })
}));

test("uses the platform keyboard layout and dismissal behavior", () => {
  let renderer: ReactTestRenderer;
  act(() => {
    renderer = create(
      <MobileKeyboardAvoidingView>
        <Text>content</Text>
      </MobileKeyboardAvoidingView>
    );
  });

  const avoidingView = renderer!.root.findByType(KeyboardAvoidingView);
  expect(avoidingView.props.behavior).toBe(
    Platform.OS === "ios" ? "padding" : "height"
  );
  expect(avoidingView.props.keyboardVerticalOffset).toBe(24);
  expect(mobileKeyboardDismissMode).toBe(
    Platform.OS === "ios" ? "interactive" : "on-drag"
  );
});

test("allows full-screen overlays to override the safe-area offset", () => {
  let renderer: ReactTestRenderer;
  act(() => {
    renderer = create(
      <MobileKeyboardAvoidingView keyboardVerticalOffset={0}>
        <Text>content</Text>
      </MobileKeyboardAvoidingView>
    );
  });

  const avoidingView = renderer!.root.findByType(KeyboardAvoidingView);
  expect(avoidingView.props.keyboardVerticalOffset).toBe(0);
});
