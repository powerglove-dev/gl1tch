import { StrictMode, Component } from "react";
import type { ReactNode, ErrorInfo } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";
import "./global.css";

class ErrorBoundary extends Component<{ children: ReactNode }, { error: string | null }> {
  state = { error: null as string | null };
  static getDerivedStateFromError(error: Error) {
    return { error: error.message + "\n" + error.stack };
  }
  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("gl1tch crash:", error, info);
  }
  render() {
    if (this.state.error) {
      return (
        <div style={{ padding: 20, color: "#f7768e", background: "#1a1b26", fontFamily: "monospace", fontSize: 12, whiteSpace: "pre-wrap" }}>
          <h2 style={{ color: "#ff9e64" }}>gl1tch crashed</h2>
          <p>{this.state.error}</p>
        </div>
      );
    }
    return this.props.children;
  }
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ErrorBoundary>
      <App />
    </ErrorBoundary>
  </StrictMode>,
);
