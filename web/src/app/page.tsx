import { Link } from "@heroui/react";
import Navbar from "@/components/Navbar";

export default function Home() {
  return (
    <div className="min-h-screen bg-background text-foreground">
      <Navbar />
      <main className="container mx-auto px-6 py-16">
        <div className="flex flex-col items-center justify-center gap-8 text-center">
          <h1 className="text-5xl font-bold tracking-tight text-primary">
            Fleet
          </h1>
          <p className="text-xl text-default-500 max-w-2xl">
            微服务管理平台 — 统一的服务目录、部署中心、发布审批与集群运维
          </p>
          <div className="flex flex-wrap gap-4 justify-center mt-8">
            <Link
              href="/services"
              color="primary"
              size="lg"
              showUnderline
            >
              服务目录 →
            </Link>
            <Link
              href="/deployments"
              color="primary"
              size="lg"
              showUnderline
            >
              部署中心 →
            </Link>
            <Link
              href="/clusters"
              color="primary"
              size="lg"
              showUnderline
            >
              集群运维 →
            </Link>
          </div>
        </div>
      </main>
    </div>
  );
}
