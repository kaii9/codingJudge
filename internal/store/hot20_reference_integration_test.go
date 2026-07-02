//go:build integration

package store

import (
	"bufio"
	"container/heap"
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
)

func TestHot20SeedExpectedOutputs(t *testing.T) {
	st := integrationStore(t)
	rows, err := st.pool.Query(context.Background(), `
		SELECT tc.problem_id, tc.input, tc.expected_output
		FROM problem_test_cases tc
		JOIN problems p ON p.id = tc.problem_id
		WHERE p.collection = 'hot20'
		ORDER BY tc.problem_id, tc.id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	validated := 0
	for rows.Next() {
		var problemID, input, expected string
		if err := rows.Scan(&problemID, &input, &expected); err != nil {
			t.Fatal(err)
		}
		actual, err := solveHot20Reference(problemID, input)
		if err != nil {
			t.Fatalf("%s case %d: %v", problemID, validated+1, err)
		}
		if normalizeReferenceOutput(actual) != normalizeReferenceOutput(expected) {
			t.Fatalf("%s input %q: got %q want %q", problemID, input, actual, expected)
		}
		validated++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if validated != 120 {
		t.Fatalf("validated %d cases, want 120", validated)
	}
}

func solveHot20Reference(problemID, input string) (string, error) {
	switch problemID {
	case "target-pair":
		return solveTargetPair(input), nil
	case "consecutive-streak":
		return solveConsecutiveStreak(input), nil
	case "product-without-self":
		return solveProductWithoutSelf(input), nil
	case "widest-water-container":
		return solveWidestWaterContainer(input), nil
	case "zero-sum-triples":
		return solveZeroSumTriples(input), nil
	case "unique-character-window":
		return solveUniqueCharacterWindow(input), nil
	case "smallest-covering-segment":
		return solveSmallestCoveringSegment(input), nil
	case "balanced-delimiters":
		return solveBalancedDelimiters(input), nil
	case "warmer-day-distance":
		return solveWarmerDayDistance(input), nil
	case "largest-skyline-rectangle":
		return solveLargestSkylineRectangle(input), nil
	case "reverse-node-chain":
		values := scanInt64Values(input)
		reverse(values)
		return formatInt64Line(values), nil
	case "merge-sorted-chains":
		return solveMergeSortedChains(input), nil
	case "cycle-entry":
		return solveCycleEntry(input), nil
	case "tree-level-traversal":
		return solveTreeLevelTraversal(input), nil
	case "search-tree-validation":
		return solveSearchTreeValidation(input), nil
	case "archipelago-count":
		return solveArchipelagoCount(input), nil
	case "course-dependency-order":
		return solveCourseDependencyOrder(input), nil
	case "unique-target-combinations":
		return solveUniqueTargetCombinations(input), nil
	case "maximum-non-adjacent-sum":
		return solveMaximumNonAdjacentSum(input), nil
	case "minimum-coin-count":
		return solveMinimumCoinCount(input), nil
	default:
		return "", fmt.Errorf("unknown reference solver %q", problemID)
	}
}

func normalizeReferenceOutput(output string) string {
	return strings.TrimRight(output, " \t\r\n")
}

func scanInt64Values(input string) []int64 {
	r := strings.NewReader(input)
	var n int
	fmt.Fscan(r, &n)
	values := make([]int64, n)
	for i := range values {
		fmt.Fscan(r, &values[i])
	}
	return values
}

func formatInt64Line(values []int64) string {
	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = fmt.Sprint(value)
	}
	return strings.Join(parts, " ") + "\n"
}

func reverse(values []int64) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func solveTargetPair(input string) string {
	r := strings.NewReader(input)
	var n int
	var target int64
	fmt.Fscan(r, &n, &target)
	values := make([]int64, n)
	for i := range values {
		fmt.Fscan(r, &values[i])
	}
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if values[i]+values[j] == target {
				return fmt.Sprintf("%d %d\n", i, j)
			}
		}
	}
	return "-1 -1\n"
}

func solveConsecutiveStreak(input string) string {
	values := scanInt64Values(input)
	set := make(map[int64]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	best := 0
	for value := range set {
		if _, exists := set[value-1]; exists {
			continue
		}
		length := 1
		for next := value + 1; ; next++ {
			if _, exists := set[next]; !exists {
				break
			}
			length++
		}
		if length > best {
			best = length
		}
	}
	return fmt.Sprintf("%d\n", best)
}

func solveProductWithoutSelf(input string) string {
	values := scanInt64Values(input)
	result := make([]int64, len(values))
	prefix := int64(1)
	for i, value := range values {
		result[i] = prefix
		prefix *= value
	}
	suffix := int64(1)
	for i := len(values) - 1; i >= 0; i-- {
		result[i] *= suffix
		suffix *= values[i]
	}
	return formatInt64Line(result)
}

func solveWidestWaterContainer(input string) string {
	heights := scanInt64Values(input)
	left, right, best := 0, len(heights)-1, int64(0)
	for left < right {
		height := heights[left]
		if heights[right] < height {
			height = heights[right]
		}
		area := int64(right-left) * height
		if area > best {
			best = area
		}
		if heights[left] <= heights[right] {
			left++
		} else {
			right--
		}
	}
	return fmt.Sprintf("%d\n", best)
}

func solveZeroSumTriples(input string) string {
	values := scanInt64Values(input)
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	triples := make([][3]int64, 0)
	for i := 0; i+2 < len(values); i++ {
		if i > 0 && values[i] == values[i-1] {
			continue
		}
		left, right := i+1, len(values)-1
		for left < right {
			sum := values[i] + values[left] + values[right]
			switch {
			case sum < 0:
				left++
			case sum > 0:
				right--
			default:
				triples = append(triples, [3]int64{values[i], values[left], values[right]})
				left++
				right--
				for left < right && values[left] == values[left-1] {
					left++
				}
			}
		}
	}
	var out strings.Builder
	fmt.Fprintln(&out, len(triples))
	for _, triple := range triples {
		fmt.Fprintf(&out, "%d %d %d\n", triple[0], triple[1], triple[2])
	}
	return out.String()
}

func inputLine(input string) string {
	return strings.TrimSuffix(strings.TrimSuffix(input, "\n"), "\r")
}

func solveUniqueCharacterWindow(input string) string {
	line := inputLine(input)
	last := make(map[byte]int)
	left, best := 0, 0
	for right := 0; right < len(line); right++ {
		if index, exists := last[line[right]]; exists && index >= left {
			left = index + 1
		}
		last[line[right]] = right
		if length := right - left + 1; length > best {
			best = length
		}
	}
	return fmt.Sprintf("%d\n", best)
}

func solveSmallestCoveringSegment(input string) string {
	scanner := bufio.NewScanner(strings.NewReader(input))
	scanner.Scan()
	source := scanner.Text()
	scanner.Scan()
	target := scanner.Text()
	if target == "" {
		return "\n"
	}
	need := make(map[byte]int)
	for i := range target {
		need[target[i]]++
	}
	missing, left, bestStart, bestLength := len(target), 0, 0, len(source)+1
	for right := range source {
		character := source[right]
		if need[character] > 0 {
			missing--
		}
		need[character]--
		for missing == 0 {
			if length := right - left + 1; length < bestLength {
				bestStart, bestLength = left, length
			}
			character = source[left]
			need[character]++
			if need[character] > 0 {
				missing++
			}
			left++
		}
	}
	if bestLength > len(source) {
		return "-1\n"
	}
	return source[bestStart:bestStart+bestLength] + "\n"
}

func solveBalancedDelimiters(input string) string {
	pairs := map[byte]byte{')': '(', ']': '[', '}': '{'}
	stack := make([]byte, 0)
	for _, character := range []byte(inputLine(input)) {
		if character == '(' || character == '[' || character == '{' {
			stack = append(stack, character)
			continue
		}
		if len(stack) == 0 || stack[len(stack)-1] != pairs[character] {
			return "false\n"
		}
		stack = stack[:len(stack)-1]
	}
	return fmt.Sprintf("%t\n", len(stack) == 0)
}

func solveWarmerDayDistance(input string) string {
	temperatures := scanInt64Values(input)
	result := make([]int64, len(temperatures))
	stack := make([]int, 0)
	for i, temperature := range temperatures {
		for len(stack) > 0 && temperature > temperatures[stack[len(stack)-1]] {
			previous := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			result[previous] = int64(i - previous)
		}
		stack = append(stack, i)
	}
	return formatInt64Line(result)
}

func solveLargestSkylineRectangle(input string) string {
	heights := append(scanInt64Values(input), 0)
	stack := make([]int, 0)
	best := int64(0)
	for i, height := range heights {
		for len(stack) > 0 && height < heights[stack[len(stack)-1]] {
			index := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			left := -1
			if len(stack) > 0 {
				left = stack[len(stack)-1]
			}
			if area := heights[index] * int64(i-left-1); area > best {
				best = area
			}
		}
		stack = append(stack, i)
	}
	return fmt.Sprintf("%d\n", best)
}

func solveMergeSortedChains(input string) string {
	r := strings.NewReader(input)
	var n, m int
	fmt.Fscan(r, &n, &m)
	left, right := make([]int64, n), make([]int64, m)
	for i := range left {
		fmt.Fscan(r, &left[i])
	}
	for i := range right {
		fmt.Fscan(r, &right[i])
	}
	merged := make([]int64, 0, n+m)
	for i, j := 0, 0; i < n || j < m; {
		if j == m || (i < n && left[i] <= right[j]) {
			merged = append(merged, left[i])
			i++
		} else {
			merged = append(merged, right[j])
			j++
		}
	}
	return formatInt64Line(merged)
}

func solveCycleEntry(input string) string {
	r := strings.NewReader(input)
	var n int
	fmt.Fscan(r, &n)
	next := make([]int, n)
	for i := range next {
		fmt.Fscan(r, &next[i])
	}
	seen := make(map[int]struct{})
	for current := 0; current >= 0 && current < n; current = next[current] {
		if _, exists := seen[current]; exists {
			return fmt.Sprintf("%d\n", current)
		}
		seen[current] = struct{}{}
	}
	return "-1\n"
}

type referenceTreeNode struct {
	value       int64
	left, right *referenceTreeNode
}

func parseReferenceTree(input string) *referenceTreeNode {
	r := strings.NewReader(input)
	var n int
	fmt.Fscan(r, &n)
	if n == 0 {
		return nil
	}
	tokens := make([]string, n)
	for i := range tokens {
		fmt.Fscan(r, &tokens[i])
	}
	if tokens[0] == "null" {
		return nil
	}
	var rootValue int64
	fmt.Sscan(tokens[0], &rootValue)
	root := &referenceTreeNode{value: rootValue}
	queue := []*referenceTreeNode{root}
	for tokenIndex := 1; tokenIndex < n && len(queue) > 0; {
		node := queue[0]
		queue = queue[1:]
		if tokens[tokenIndex] != "null" {
			var value int64
			fmt.Sscan(tokens[tokenIndex], &value)
			node.left = &referenceTreeNode{value: value}
			queue = append(queue, node.left)
		}
		tokenIndex++
		if tokenIndex < n && tokens[tokenIndex] != "null" {
			var value int64
			fmt.Sscan(tokens[tokenIndex], &value)
			node.right = &referenceTreeNode{value: value}
			queue = append(queue, node.right)
		}
		tokenIndex++
	}
	return root
}

func solveTreeLevelTraversal(input string) string {
	root := parseReferenceTree(input)
	if root == nil {
		return "0\n"
	}
	levels := make([][]int64, 0)
	queue := []*referenceTreeNode{root}
	for len(queue) > 0 {
		count := len(queue)
		level := make([]int64, 0, count)
		for i := 0; i < count; i++ {
			node := queue[0]
			queue = queue[1:]
			level = append(level, node.value)
			if node.left != nil {
				queue = append(queue, node.left)
			}
			if node.right != nil {
				queue = append(queue, node.right)
			}
		}
		levels = append(levels, level)
	}
	var out strings.Builder
	fmt.Fprintln(&out, len(levels))
	for _, level := range levels {
		out.WriteString(formatInt64Line(level))
	}
	return out.String()
}

func solveSearchTreeValidation(input string) string {
	var validate func(*referenceTreeNode, *int64, *int64) bool
	validate = func(node *referenceTreeNode, minimum, maximum *int64) bool {
		if node == nil {
			return true
		}
		if minimum != nil && node.value <= *minimum || maximum != nil && node.value >= *maximum {
			return false
		}
		return validate(node.left, minimum, &node.value) && validate(node.right, &node.value, maximum)
	}
	return fmt.Sprintf("%t\n", validate(parseReferenceTree(input), nil, nil))
}

func solveArchipelagoCount(input string) string {
	r := strings.NewReader(input)
	var rows, columns int
	fmt.Fscan(r, &rows, &columns)
	grid := make([][]byte, rows)
	for i := range grid {
		var line string
		fmt.Fscan(r, &line)
		grid[i] = []byte(line)
	}
	count := 0
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			if grid[row][column] != '1' {
				continue
			}
			count++
			grid[row][column] = '0'
			queue := [][2]int{{row, column}}
			for len(queue) > 0 {
				cell := queue[0]
				queue = queue[1:]
				for _, delta := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
					nextRow, nextColumn := cell[0]+delta[0], cell[1]+delta[1]
					if nextRow >= 0 && nextRow < rows && nextColumn >= 0 && nextColumn < columns && grid[nextRow][nextColumn] == '1' {
						grid[nextRow][nextColumn] = '0'
						queue = append(queue, [2]int{nextRow, nextColumn})
					}
				}
			}
		}
	}
	return fmt.Sprintf("%d\n", count)
}

type integerHeap []int

func (h integerHeap) Len() int           { return len(h) }
func (h integerHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h integerHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *integerHeap) Push(value any)    { *h = append(*h, value.(int)) }
func (h *integerHeap) Pop() any {
	old := *h
	value := old[len(old)-1]
	*h = old[:len(old)-1]
	return value
}

func solveCourseDependencyOrder(input string) string {
	r := strings.NewReader(input)
	var n, m int
	fmt.Fscan(r, &n, &m)
	graph := make([][]int, n)
	indegree := make([]int, n)
	for i := 0; i < m; i++ {
		var course, prerequisite int
		fmt.Fscan(r, &course, &prerequisite)
		graph[prerequisite] = append(graph[prerequisite], course)
		indegree[course]++
	}
	ready := &integerHeap{}
	heap.Init(ready)
	for course, degree := range indegree {
		if degree == 0 {
			heap.Push(ready, course)
		}
	}
	order := make([]int64, 0, n)
	for ready.Len() > 0 {
		course := heap.Pop(ready).(int)
		order = append(order, int64(course))
		for _, dependent := range graph[course] {
			indegree[dependent]--
			if indegree[dependent] == 0 {
				heap.Push(ready, dependent)
			}
		}
	}
	if len(order) != n {
		return "-1\n"
	}
	return formatInt64Line(order)
}

func solveUniqueTargetCombinations(input string) string {
	r := strings.NewReader(input)
	var n, target int
	fmt.Fscan(r, &n, &target)
	candidates := make([]int, n)
	for i := range candidates {
		fmt.Fscan(r, &candidates[i])
	}
	sort.Ints(candidates)
	combinations := make([][]int, 0)
	var search func(int, int, []int)
	search = func(start, remaining int, current []int) {
		if remaining == 0 {
			combinations = append(combinations, append([]int(nil), current...))
			return
		}
		for i := start; i < len(candidates) && candidates[i] <= remaining; i++ {
			search(i, remaining-candidates[i], append(current, candidates[i]))
		}
	}
	search(0, target, nil)
	var out strings.Builder
	fmt.Fprintln(&out, len(combinations))
	for _, combination := range combinations {
		parts := make([]string, len(combination))
		for i, value := range combination {
			parts[i] = fmt.Sprint(value)
		}
		out.WriteString(strings.Join(parts, " ") + "\n")
	}
	return out.String()
}

func solveMaximumNonAdjacentSum(input string) string {
	values := scanInt64Values(input)
	previous, current := int64(0), int64(0)
	for _, value := range values {
		next := current
		if candidate := previous + value; candidate > next {
			next = candidate
		}
		previous, current = current, next
	}
	return fmt.Sprintf("%d\n", current)
}

func solveMinimumCoinCount(input string) string {
	r := strings.NewReader(input)
	var n, amount int
	fmt.Fscan(r, &n, &amount)
	coins := make([]int, n)
	for i := range coins {
		fmt.Fscan(r, &coins[i])
	}
	dp := make([]int, amount+1)
	for value := 1; value <= amount; value++ {
		dp[value] = amount + 1
		for _, coin := range coins {
			if coin <= value && dp[value-coin]+1 < dp[value] {
				dp[value] = dp[value-coin] + 1
			}
		}
	}
	if dp[amount] > amount {
		return "-1\n"
	}
	return fmt.Sprintf("%d\n", dp[amount])
}
