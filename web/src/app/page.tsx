import Link from "next/link";
import Navbar from "@/components/Navbar";

export default function Home() {
  return (
    <div className="min-h-screen bg-white text-gray-900 dark:bg-gray-950 dark:text-gray-100">
      <Navbar />
      <main className="container mx-auto px-6 py-16">
        <div className="flex flex-col items-center justify-center gap-8 text-center">
          <h1 className="text-5xl font-bold tracking-tight text-blue-600">
            Fleet
          </h1>
          <p className="text-xl text-gray-500 max-w-2xl dark:text-gray-400">
            微服务管理平台 — 统一的服务目录、部署中心、发布审批与集群运维
          </p>
          <div className="flex flex-wrap gap-4 justify-center mt-8">
            <Link
              href="/services"
              className="text-lg font-medium text-blue-600 hover:underline"
            >
              服务目录 →
            </Link>
            <Link
              href="/deployments"
              className="text-lg font-medium text-blue-600 hover:underline"
            >
              部署中心 →
            </Link>
            <Link
              href="/clusters"
              className="text-lg font-medium text-blue-600 hover:underline"
            >
              集群运维 →
            </Link>
          </div>
        </div>
      </main>
    </div>
  );
}
