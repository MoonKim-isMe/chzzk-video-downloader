export type AuthenticationMode = 'browser' | 'cookies_file';

export type AuthenticationBrowser =
  | 'chrome'
  | 'edge'
  | 'whale'
  | 'firefox'
  | 'brave'
  | 'vivaldi';

export interface AuthenticationSettings {
  enabled: boolean;
  mode: AuthenticationMode;
  browser: AuthenticationBrowser;
  browserProfile: string;
  cookiesFilePath: string;
}
