import { BrowserRouter, Routes, Route, Link } from "react-router-dom";
import Home from "@/pages/home";
import About from "@/pages/about";

function App() {
  return (
    <BrowserRouter>
      {/* ส่วนนี้คือแถบเมนู (Navbar) ที่จะแสดงอยู่ทุกหน้า */}
      <nav className="p-4 border-b bg-card flex gap-4 justify-center">
        <Link to="/" className="font-medium hover:text-blue-600 transition-colors">
          หน้าแรก
        </Link>
        <Link to="/about" className="font-medium hover:text-blue-600 transition-colors">
          เกี่ยวกับเรา
        </Link>
      </nav>
      <main className="min-h-screen bg-background">
        <Routes>
          <Route path="/" element={<Home />} />
          <Route path="/about" element={<About />} />
        </Routes>
      </main>
    </BrowserRouter>
  );
}

export default App;