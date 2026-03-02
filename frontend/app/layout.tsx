import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "Kuberless",
  description: "Deploy your containers, they just work",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en">
      <body className="bg-gray-50 text-gray-900 antialiased">{children}</body>
    </html>
  );
}
