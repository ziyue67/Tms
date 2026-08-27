import { Navbar } from "@/components/navbar";

export default function DefaultLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <div className="relative flex min-h-screen w-full min-w-0 flex-col bg-white dark:bg-black">
      <Navbar />
      <main className="container mx-auto w-full min-w-0 max-w-7xl flex-grow px-3 pt-4 sm:px-6 sm:pt-16">
        {children}
      </main>
    </div>
  );
}
