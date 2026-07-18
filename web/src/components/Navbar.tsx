"use client";

import { useState } from "react";
import Link from "next/link";
import {
  Navbar as HeroUINavbar,
  NavbarContent,
  NavbarMenu,
  NavbarMenuItem,
  NavbarMenuToggle,
  NavbarBrand,
  NavbarItem,
} from "@heroui/react";

const navItems = [
  { label: "服务目录", href: "/services" },
  { label: "部署中心", href: "/deployments" },
  { label: "发布审批", href: "/approvals" },
  { label: "集群运维", href: "/clusters" },
  { label: "审计分析", href: "/audit" },
  { label: "系统设置", href: "/settings" },
];

export default function Navbar() {
  const [isMenuOpen, setIsMenuOpen] = useState(false);

  return (
    <HeroUINavbar
      isMenuOpen={isMenuOpen}
      onMenuOpenChange={setIsMenuOpen}
      maxWidth="xl"
    >
      <NavbarContent justify="start">
        <NavbarMenuToggle
          aria-label={isMenuOpen ? "关闭菜单" : "打开菜单"}
          className="sm:hidden"
        />
        <NavbarBrand>
          <p className="text-xl font-bold text-primary">Fleet</p>
        </NavbarBrand>
      </NavbarContent>

      <NavbarContent className="hidden sm:flex gap-4" justify="center">
        {navItems.map((item) => (
          <NavbarItem key={item.href}>
            <Link
              href={item.href}
              className="text-sm font-medium hover:text-primary transition-colors"
            >
              {item.label}
            </Link>
          </NavbarItem>
        ))}
      </NavbarContent>

      <NavbarContent justify="end">
        <NavbarItem>
          <div className="w-8 h-8 rounded-full bg-primary text-white flex items-center justify-center text-sm font-medium">
            U
          </div>
        </NavbarItem>
      </NavbarContent>

      <NavbarMenu>
        {navItems.map((item, index) => (
          <NavbarMenuItem key={`${item.href}-${index}`}>
            <Link
              href={item.href}
              className="w-full"
              onClick={() => setIsMenuOpen(false)}
            >
              {item.label}
            </Link>
          </NavbarMenuItem>
        ))}
      </NavbarMenu>
    </HeroUINavbar>
  );
}
