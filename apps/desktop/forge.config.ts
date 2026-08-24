import type { ForgeConfig } from "@electron-forge/shared-types";
import { FusesPlugin } from "@electron-forge/plugin-fuses";
import { VitePlugin } from "@electron-forge/plugin-vite";
import { FuseV1Options, FuseVersion } from "@electron/fuses";
import { MakerDeb } from "@electron-forge/maker-deb";
import { MakerDMG } from "@electron-forge/maker-dmg";
import { MakerRpm } from "@electron-forge/maker-rpm";
import { MakerSquirrel } from "@electron-forge/maker-squirrel";
import { MakerZIP } from "@electron-forge/maker-zip";

const releaseSigningEnabled = process.env.CONTENTCLOUD_DESKTOP_SIGN === "1";
const desktopProductName = "Content Work OS";
const desktopExecutableName = "content-work-os";
const desktopSquirrelName = "content_work_os";
const linuxMakerOptions = {
  name: desktopExecutableName,
  productName: desktopProductName,
  bin: desktopExecutableName,
};

function macSignOptions() {
  if (process.platform !== "darwin" || !releaseSigningEnabled) {
    return undefined;
  }

  const identity = process.env.APPLE_SIGNING_IDENTITY?.trim();
  if (!identity) {
    throw new Error(
      "APPLE_SIGNING_IDENTITY is required for a signed desktop release",
    );
  }

  return {
    identity,
    keychain: process.env.CONTENTCLOUD_DESKTOP_MAC_KEYCHAIN,
    hardenedRuntime: true,
    gatekeeperAssess: true,
    identityValidation: true,
  };
}

function macNotarizeOptions() {
  if (process.platform !== "darwin" || !releaseSigningEnabled) {
    return undefined;
  }

  const appleId = process.env.APPLE_ID?.trim();
  const appleIdPassword = process.env.APPLE_APP_SPECIFIC_PASSWORD?.trim();
  const teamId = process.env.APPLE_TEAM_ID?.trim();
  if (!appleId || !appleIdPassword || !teamId) {
    throw new Error(
      "APPLE_ID, APPLE_APP_SPECIFIC_PASSWORD and APPLE_TEAM_ID are required for notarization",
    );
  }

  return { appleId, appleIdPassword, teamId };
}

function squirrelOptions() {
  const identity = { name: desktopSquirrelName };
  if (process.platform !== "win32" || !releaseSigningEnabled) {
    return identity;
  }

  const certificateFile =
    process.env.CONTENTCLOUD_DESKTOP_WINDOWS_CERTIFICATE_FILE?.trim();
  const certificatePassword =
    process.env.CONTENTCLOUD_DESKTOP_WINDOWS_CERTIFICATE_PASSWORD;
  if (!certificateFile || !certificatePassword) {
    throw new Error(
      "CONTENTCLOUD_DESKTOP_WINDOWS_CERTIFICATE_FILE and CONTENTCLOUD_DESKTOP_WINDOWS_CERTIFICATE_PASSWORD are required for a signed desktop release",
    );
  }

  return { ...identity, certificateFile, certificatePassword };
}

const config: ForgeConfig = {
  packagerConfig: {
    asar: true,
    name: desktopProductName,
    executableName: desktopExecutableName,
    appBundleId: "run.zhongcao.contentcloud.desktop",
    protocols: [{ name: desktopProductName, schemes: ["contentcloud"] }],
    osxSign: macSignOptions(),
    osxNotarize: macNotarizeOptions(),
    win32metadata: {
      CompanyName: "ContentCloud",
      ProductName: desktopProductName,
      FileDescription: "ContentCloud project workspace desktop",
    },
  },
  rebuildConfig: {},
  makers: [
    new MakerSquirrel(squirrelOptions()),
    new MakerZIP({}, ["darwin"]),
    new MakerDMG({}),
    new MakerDeb({ options: linuxMakerOptions }),
    new MakerRpm({ options: linuxMakerOptions }),
  ],
  plugins: [
    new VitePlugin({
      build: [
        {
          entry: "src/main/main.ts",
          config: "vite.main.config.ts",
          target: "main",
        },
        {
          entry: "src/preload/preload.ts",
          config: "vite.preload.config.ts",
          target: "preload",
        },
      ],
      renderer: [{ name: "main_window", config: "vite.renderer.config.ts" }],
    }),
    new FusesPlugin({
      version: FuseVersion.V1,
      [FuseV1Options.RunAsNode]: false,
      [FuseV1Options.EnableCookieEncryption]: true,
      [FuseV1Options.EnableNodeOptionsEnvironmentVariable]: false,
      [FuseV1Options.EnableNodeCliInspectArguments]: false,
      [FuseV1Options.EnableEmbeddedAsarIntegrityValidation]: true,
      [FuseV1Options.OnlyLoadAppFromAsar]: true,
    }),
  ],
};

export default config;
