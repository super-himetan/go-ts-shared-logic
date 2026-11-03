import { Limit } from "../../tests/testdata/constsonly/limits.go";
import { Bump } from "../../tests/testdata/multi/x.go";
import { Count } from "../../tests/testdata/multi/y.go";

Bump();
Bump();
document.getElementById("result").textContent = `count=${Count()} limit=${Limit}`;
