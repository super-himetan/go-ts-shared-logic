import { goWasm } from "go-mapper/vite";

export default {
  root: "e2e/multi",
  plugins: [goWasm()],
  build: { emptyOutDir: true },
};
