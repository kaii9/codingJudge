// k6-loadtest — accepted programs for each supported language.
// These all compute the sum of two integers from stdin.

export const go = `
package main
import "fmt"
func main() {
  var a, b int
  fmt.Scan(&a, &b)
  fmt.Println(a + b)
}
// k6-loadtest
`.trim();

export const cpp = `
#include <iostream>
int main() {
  int a, b;
  std::cin >> a >> b;
  std::cout << a + b << std::endl;
  return 0;
}
// k6-loadtest
`.trim();

export const python = `
a, b = map(int, input().split())
print(a + b)
# k6-loadtest
`.trim();

export function byLanguage(lang) {
  switch (lang) {
    case 'go': return go;
    case 'cpp': return cpp;
    case 'python': return python;
    default: return go;
  }
}
