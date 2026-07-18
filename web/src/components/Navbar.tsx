"use client";

import {
  Navbar as HeroUINavbar,
  NavbarContent,
  NavbarMenu,
  NavbarMenuItem,
  NavbarMenuToggle,
  NavbarBrand,
  NavbarItem,
  Link,
  Avatar,
  Divider,
} from "@heroui/react";
import { useState } from "react";

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
      className="border-b border-divider"
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
              color="foreground"
              className="text-sm font-medium hover:text-primary transition-colors"
            >
              {item.label}
            </Link>
          </NavbarItem>
        ))}
      </NavbarContent>

      <NavbarContent justify="end">
        <NavbarItem>
          <Avatar
            name="U"
            size="sm"
            color="primary"
            isBordered
            className="cursor-pointer"
          />
        </NavbarItem>
      </NavbarContent>

      <NavbarMenu>
        {navItems.map((item, index) => (
          <NavbarMenuItem key={`${item.href}-${index}`}>
            <Link
              href={item.href}
              color="foreground"
              className="w-full"
              onPress={() => setIsMenuOpen(false)}
            >
              {item.label}
            </Link>
          </NavbarMenuItem>
        ))}
        <Divider className="my-2" />
        <NavbarMenuItem>
          <Link href="/profile" color="foreground" className="w-full">
            个人中心
          </Link>
        </NavbarMenuItem>
        <NavbarMenuItem>
          <Link href="/logout" color="danger" className="w-full">
            退出登录
          </Link>
        </NavbarMenuItem>
      </NavbarMenu>
    </HeroUINavbar>
  );
}
