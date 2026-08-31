import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Button } from "./button";
import { Input } from "./input";
import { Textarea } from "./textarea";

describe("dashboard controls", () => {
  it("uses Supabase dashboard compact button metrics by default", () => {
    render(<Button>Save changes</Button>);

    expect(screen.getByRole("button", { name: "Save changes" })).toHaveClass(
      "h-[26px]",
      "px-2.5",
      "text-[12px]",
      "leading-[16px]",
      "font-[450]",
    );
  });

  it("uses Inter typography for editable values and placeholders", () => {
    render(
      <>
        <Input placeholder="project.example.com" />
        <Textarea placeholder="One URL per line" />
      </>,
    );

    expect(screen.getByPlaceholderText("project.example.com")).toHaveClass(
      "font-sans",
      "text-[13px]",
      "leading-[16px]",
      "font-[450]",
      "placeholder:text-muted-foreground",
    );
    expect(screen.getByPlaceholderText("One URL per line")).toHaveClass(
      "font-sans",
      "text-[13px]",
      "leading-[16px]",
      "font-[450]",
      "placeholder:text-muted-foreground",
    );
  });
});
