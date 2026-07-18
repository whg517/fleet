"use client";

import { HeroUIProvider } from "@heroui/react";
import { ThemeProvider as NextThemesProvider } from "next-themes";
import type { ThemeProviderProps } from "next-themes/dist/types";

export function Providers({ children }: ThemeProviderProps) {
  return (
    <NextThemesProvider attribute="class" defaultTheme="light">
      <HeroUIProvider>{children}</HeroUIProvider>
    </NextThemesProvider>
  );
}
