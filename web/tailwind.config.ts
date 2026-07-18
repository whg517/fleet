import type { Config } from "tailwindcss";
import { heroui } from "@heroui/theme";

const config: Config = {
  content: [
    "./src/**/*.{js,ts,jsx,tsx,mdx}",
    "./node_modules/@heroui/theme/dist/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {},
  },
  darkMode: "class",
  plugins: [
    heroui({
      themes: {
        light: {
          colors: {
            background: "#ffffff",
            foreground: "#11181C",
            primary: {
              50: "#e6f0ff",
              100: "#b3d1ff",
              200: "#80b3ff",
              300: "#4d94ff",
              400: "#1a75ff",
              500: "#0052cc",
              600: "#004299",
              700: "#003166",
              800: "#002133",
              900: "#001100",
            },
          },
        },
        dark: {
          colors: {
            background: "#1a1a1a",
            foreground: "#ececec",
          },
        },
      },
    }),
  ],
};

export default config;
