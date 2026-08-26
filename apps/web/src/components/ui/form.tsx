"use client"

import * as React from "react"
import { Controller, FormProvider, useFormContext, useFormState, type ControllerProps, type FieldPath, type FieldValues } from "react-hook-form"
import { cn } from "@/lib/utils"
import { Label } from "@/components/ui/label"

export const Form = FormProvider
const FormFieldContext = React.createContext<{ name: string }>({ name: "" })
export function FormField<TFieldValues extends FieldValues = FieldValues, TName extends FieldPath<TFieldValues> = FieldPath<TFieldValues>>(props: ControllerProps<TFieldValues, TName>) { return <FormFieldContext.Provider value={{ name: props.name }}><Controller {...props} /></FormFieldContext.Provider> }
export function useFormField() { const field = React.useContext(FormFieldContext); const { getFieldState } = useFormContext(); const state = useFormState({ name: field.name }); return { ...getFieldState(field.name, state), name: field.name, formItemId: `${field.name}-form-item`, formDescriptionId: `${field.name}-form-item-description`, formMessageId: `${field.name}-form-item-message` } }
export function FormItem({ className, ...props }: React.ComponentProps<'div'>) { const { error } = useFormField(); return <div className={cn('space-y-2', className)} data-invalid={!!error} {...props} /> }
export function FormLabel({ className, ...props }: React.ComponentProps<typeof Label>) { const { error, formItemId } = useFormField(); return <Label className={cn(error && 'text-destructive', className)} htmlFor={formItemId} {...props} /> }
export function FormControl({ children, ...props }: { children: React.ReactElement } & React.HTMLAttributes<HTMLElement>) { const { error, formItemId, formDescriptionId, formMessageId } = useFormField(); return React.cloneElement(children as React.ReactElement<Record<string, unknown>>, { ...props, id: formItemId, 'aria-describedby': !error ? formDescriptionId : `${formDescriptionId} ${formMessageId}`, 'aria-invalid': !!error }) }
export function FormDescription({ className, ...props }: React.ComponentProps<'p'>) { const { formDescriptionId } = useFormField(); return <p id={formDescriptionId} className={cn('text-sm text-muted-foreground', className)} {...props} /> }
export function FormMessage({ className, children, ...props }: React.ComponentProps<'p'>) { const { error, formMessageId } = useFormField(); const body = error ? String(error.message) : children; if (!body) return null; return <p id={formMessageId} role="alert" className={cn('text-sm font-normal text-destructive', className)} {...props}>{body}</p> }
