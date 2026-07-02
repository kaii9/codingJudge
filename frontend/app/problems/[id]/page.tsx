import { Workbench } from "@/components/workbench";

export default async function ProblemPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return <Workbench problemId={id} />;
}
